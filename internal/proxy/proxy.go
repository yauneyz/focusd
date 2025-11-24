package proxy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"focusd/internal/sni"
	"golang.org/x/sys/unix"
)

const (
	// Socket options for transparent proxying
	SO_ORIGINAL_DST = 80
	IP_TRANSPARENT  = 19
	SO_MARK         = 36

	// Proxy ports
	HTTPPort  = 50080
	HTTPSPort = 50443

	// Firewall mark for proxy's own connections (prevents routing loops)
	ProxyMark = 50

	// Timeouts
	ReadTimeout    = 30 * time.Second
	WriteTimeout   = 30 * time.Second
	ForwardTimeout = 5 * time.Minute
)

// TransparentProxy implements a transparent HTTP/HTTPS proxy with SNI inspection
type TransparentProxy struct {
	blockedDomains []string
	httpListener   net.Listener
	httpsListener  net.Listener
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
}

// New creates a new transparent proxy
func New(blockedDomains []string) *TransparentProxy {
	ctx, cancel := context.WithCancel(context.Background())
	return &TransparentProxy{
		blockedDomains: blockedDomains,
		ctx:            ctx,
		cancel:         cancel,
	}
}

// Start starts the transparent proxy servers
func (p *TransparentProxy) Start() error {
	// Start HTTP proxy
	httpListener, err := p.createTransparentListener(HTTPPort)
	if err != nil {
		return fmt.Errorf("creating HTTP listener: %w", err)
	}
	p.httpListener = httpListener

	// Start HTTPS proxy
	httpsListener, err := p.createTransparentListener(HTTPSPort)
	if err != nil {
		p.httpListener.Close()
		return fmt.Errorf("creating HTTPS listener: %w", err)
	}
	p.httpsListener = httpsListener

	// Start accepting connections
	p.wg.Add(2)
	go p.acceptLoop(p.httpListener, p.handleHTTP)
	go p.acceptLoop(p.httpsListener, p.handleHTTPS)

	log.Printf("Transparent proxy started: HTTP=%d, HTTPS=%d", HTTPPort, HTTPSPort)
	return nil
}

// Stop stops the transparent proxy
func (p *TransparentProxy) Stop() error {
	log.Println("Stopping transparent proxy...")
	p.cancel()

	if p.httpListener != nil {
		p.httpListener.Close()
	}
	if p.httpsListener != nil {
		p.httpsListener.Close()
	}

	// Wait for all connections to finish (with timeout)
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("Transparent proxy stopped cleanly")
	case <-time.After(10 * time.Second):
		log.Println("Transparent proxy stopped (timeout waiting for connections)")
	}

	return nil
}

// createTransparentListener creates a transparent socket listener
func (p *TransparentProxy) createTransparentListener(port int) (net.Listener, error) {
	// Create socket
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
	if err != nil {
		return nil, fmt.Errorf("creating socket: %w", err)
	}

	// Set socket options
	if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1); err != nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("setting SO_REUSEADDR: %w", err)
	}

	if err := syscall.SetsockoptInt(fd, syscall.SOL_IP, IP_TRANSPARENT, 1); err != nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("setting IP_TRANSPARENT: %w", err)
	}

	// Bind to port
	addr := syscall.SockaddrInet4{Port: port}
	if err := syscall.Bind(fd, &addr); err != nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("binding to port %d: %w", port, err)
	}

	// Listen
	if err := syscall.Listen(fd, syscall.SOMAXCONN); err != nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("listening: %w", err)
	}

	// Convert fd to net.Listener
	file := os.NewFile(uintptr(fd), fmt.Sprintf("transparent-listener-%d", port))
	listener, err := net.FileListener(file)
	file.Close()
	if err != nil {
		return nil, fmt.Errorf("creating listener from fd: %w", err)
	}

	return listener, nil
}

// acceptLoop accepts connections and handles them
func (p *TransparentProxy) acceptLoop(listener net.Listener, handler func(net.Conn)) {
	defer p.wg.Done()

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-p.ctx.Done():
				return
			default:
				log.Printf("Accept error: %v", err)
				continue
			}
		}

		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			handler(conn)
		}()
	}
}

// handleHTTP handles HTTP connections
func (p *TransparentProxy) handleHTTP(clientConn net.Conn) {
	defer clientConn.Close()

	// Set timeouts
	clientConn.SetReadDeadline(time.Now().Add(ReadTimeout))

	// Get original destination
	origDst, err := getOriginalDst(clientConn)
	if err != nil {
		log.Printf("HTTP: Failed to get original destination: %v", err)
		return
	}

	// Read HTTP request
	reader := bufio.NewReader(clientConn)
	requestLine, err := reader.ReadString('\n')
	if err != nil {
		log.Printf("HTTP: Failed to read request line: %v", err)
		return
	}

	// Parse HTTP headers to find Host
	var host string
	for {
		line, err := reader.ReadString('\n')
		if err != nil || line == "\r\n" || line == "\n" {
			break
		}
		if strings.HasPrefix(strings.ToLower(line), "host:") {
			host = strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
			break
		}
	}

	if host == "" {
		log.Printf("HTTP: No Host header found")
		return
	}

	// Remove port from host if present
	if idx := strings.Index(host, ":"); idx != -1 {
		host = host[:idx]
	}

	log.Printf("HTTP: %s -> %s", host, origDst)

	// Check if blocked
	if p.isBlocked(host) {
		log.Printf("HTTP: Blocked %s", host)
		// Send 403 Forbidden
		response := "HTTP/1.1 403 Forbidden\r\n" +
			"Content-Type: text/html\r\n" +
			"Connection: close\r\n" +
			"\r\n" +
			"<html><body><h1>403 Forbidden</h1><p>Blocked by focusd</p></body></html>"
		clientConn.Write([]byte(response))
		return
	}

	// Forward connection
	log.Printf("HTTP: Allowed %s", host)
	p.forwardConnection(clientConn, origDst, []byte(requestLine))
}

// handleHTTPS handles HTTPS connections with SNI inspection
func (p *TransparentProxy) handleHTTPS(clientConn net.Conn) {
	defer clientConn.Close()

	// Set timeouts
	clientConn.SetReadDeadline(time.Now().Add(ReadTimeout))

	// Get original destination
	origDst, err := getOriginalDst(clientConn)
	if err != nil {
		log.Printf("HTTPS: Failed to get original destination: %v", err)
		return
	}

	// Read TLS ClientHello (usually < 1KB, but can be up to 16KB)
	buf := make([]byte, 16384)
	n, err := clientConn.Read(buf)
	if err != nil {
		log.Printf("HTTPS: Failed to read ClientHello: %v", err)
		return
	}

	clientHello := buf[:n]

	// Extract SNI
	hostname, err := sni.ExtractSNI(clientHello)
	if err != nil {
		log.Printf("HTTPS: Failed to extract SNI: %v (blocking by default)", err)
		// Without SNI, we can't make a decision - block by default
		sendTLSAlert(clientConn)
		return
	}

	log.Printf("HTTPS: %s -> %s", hostname, origDst)

	// Check if blocked
	if p.isBlocked(hostname) {
		log.Printf("HTTPS: Blocked %s", hostname)
		sendTLSAlert(clientConn)
		return
	}

	// Forward connection
	log.Printf("HTTPS: Allowed %s", hostname)
	p.forwardConnection(clientConn, origDst, clientHello)
}

// forwardConnection forwards the connection to the original destination
func (p *TransparentProxy) forwardConnection(clientConn net.Conn, destAddr string, initialData []byte) {
	// Create outbound connection with SO_MARK to prevent routing loop
	dialer := &net.Dialer{
		Timeout: 30 * time.Second,
		Control: func(network, address string, c syscall.RawConn) error {
			var sockErr error
			err := c.Control(func(fd uintptr) {
				// Set SO_MARK to bypass nftables interception
				sockErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, SO_MARK, ProxyMark)
			})
			if err != nil {
				return err
			}
			return sockErr
		},
	}

	destConn, err := dialer.Dial("tcp", destAddr)
	if err != nil {
		log.Printf("Failed to connect to %s: %v", destAddr, err)
		return
	}
	defer destConn.Close()

	// Send initial data (HTTP request line or TLS ClientHello)
	if len(initialData) > 0 {
		if _, err := destConn.Write(initialData); err != nil {
			log.Printf("Failed to write initial data: %v", err)
			return
		}
	}

	// Bidirectional copy
	var wg sync.WaitGroup
	wg.Add(2)

	// Client -> Destination
	go func() {
		defer wg.Done()
		io.Copy(destConn, clientConn)
		destConn.(*net.TCPConn).CloseWrite()
	}()

	// Destination -> Client
	go func() {
		defer wg.Done()
		io.Copy(clientConn, destConn)
		clientConn.(*net.TCPConn).CloseWrite()
	}()

	wg.Wait()
}

// isBlocked checks if a domain is in the blocklist
func (p *TransparentProxy) isBlocked(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))

	for _, blocked := range p.blockedDomains {
		blocked = strings.ToLower(strings.TrimSuffix(blocked, "."))

		// Exact match or subdomain match
		if host == blocked || strings.HasSuffix(host, "."+blocked) {
			return true
		}

		// Also check if blocked domain has www. prefix
		if strings.HasPrefix(blocked, "www.") {
			bareBlocked := strings.TrimPrefix(blocked, "www.")
			if host == bareBlocked || strings.HasSuffix(host, "."+bareBlocked) {
				return true
			}
		}
	}

	return false
}

// getOriginalDst gets the original destination address using SO_ORIGINAL_DST
func getOriginalDst(conn net.Conn) (string, error) {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return "", fmt.Errorf("not a TCP connection")
	}

	file, err := tcpConn.File()
	if err != nil {
		return "", fmt.Errorf("getting file descriptor: %w", err)
	}
	defer file.Close()

	fd := int(file.Fd())

	// Get original destination (IPv4)
	// Read raw sockaddr structure
	var addr syscall.RawSockaddrInet4
	addrLen := uint32(syscall.SizeofSockaddrInet4)
	_, _, errno := unix.Syscall6(
		unix.SYS_GETSOCKOPT,
		uintptr(fd),
		uintptr(unix.SOL_IP),
		uintptr(SO_ORIGINAL_DST),
		uintptr(unsafe.Pointer(&addr)),
		uintptr(unsafe.Pointer(&addrLen)),
		0,
	)
	if errno != 0 {
		return "", fmt.Errorf("getsockopt SO_ORIGINAL_DST: %w", errno)
	}

	// Parse sockaddr_in structure
	// Port is in network byte order (big-endian)
	ip := net.IPv4(
		addr.Addr[0],
		addr.Addr[1],
		addr.Addr[2],
		addr.Addr[3],
	)

	// Port is stored as uint16 in network byte order, swap bytes to get host byte order
	port := (addr.Port >> 8) | (addr.Port << 8)

	return fmt.Sprintf("%s:%d", ip.String(), port), nil
}

// sendTLSAlert sends a TLS alert to close the connection gracefully
func sendTLSAlert(conn net.Conn) {
	// TLS alert: close_notify
	alert := []byte{0x15, 0x03, 0x03, 0x00, 0x02, 0x01, 0x00}
	conn.SetWriteDeadline(time.Now().Add(1 * time.Second))
	conn.Write(alert)
}
