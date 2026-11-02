package common

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/smtp"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A subprocess gives each test a fresh system root pool. No verification
// bypass is needed to exercise an SMTP server signed by our disposable CA.
func TestSMTPVerifiedTLSClient(t *testing.T) {
	if addr := os.Getenv("NEWAPI_SMTP_TLS_FIXTURE"); addr != "" {
		SMTPServer, SMTPSSLEnabled, SMTPStartTLSEnabled = "localhost", os.Getenv("NEWAPI_SMTP_TLS_MODE") == "implicit", os.Getenv("NEWAPI_SMTP_TLS_MODE") == "starttls"
		SMTPInsecureSkipVerify = false
		client, err := newSMTPClient(addr, time.Second, time.Second)
		if os.Getenv("NEWAPI_SMTP_TLS_UNTRUSTED") == "true" {
			if err == nil {
				client.Close()
				t.Fatal("untrusted certificate accepted")
			}
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()
		if err := client.Quit(); err != nil {
			t.Fatal(err)
		}
		return
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "SMTP test CA"}, DNSNames: []string{"localhost"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certFile := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"implicit", "starttls"} {
		for _, trusted := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/trusted=%t", mode, trusted), func(t *testing.T) {
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				defer listener.Close()
				done := make(chan struct{})
				go func() {
					defer close(done)
					conn, err := listener.Accept()
					if err != nil {
						return
					}
					defer conn.Close()
					_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
					tlsConfig := &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}, MinVersion: tls.VersionTLS12}
					if mode == "implicit" {
						c := tls.Server(conn, tlsConfig)
						if c.Handshake() != nil {
							return
						}
						conn = c
					}
					_, _ = fmt.Fprint(conn, "220 fixture ESMTP\r\n")
					reader := bufio.NewReader(conn)
					for {
						line, err := reader.ReadString('\n')
						if err != nil {
							return
						}
						switch {
						case strings.HasPrefix(line, "EHLO"):
							_, _ = fmt.Fprint(conn, "250-fixture\r\n250 STARTTLS\r\n")
						case strings.HasPrefix(line, "STARTTLS"):
							_, _ = fmt.Fprint(conn, "220 start TLS\r\n")
							c := tls.Server(conn, tlsConfig)
							if c.Handshake() != nil {
								return
							}
							conn = c
							reader = bufio.NewReader(conn)
						case strings.HasPrefix(line, "QUIT"):
							_, _ = fmt.Fprint(conn, "221 bye\r\n")
							return
						default:
							_, _ = fmt.Fprint(conn, "250 ok\r\n")
						}
					}
				}()
				root := certFile
				if !trusted {
					root = filepath.Join(t.TempDir(), "absent.pem")
				}
				cmd := exec.Command(exe, "-test.run=^TestSMTPVerifiedTLSClient$")
				cmd.Env = append(os.Environ(), "NEWAPI_SMTP_TLS_FIXTURE="+listener.Addr().String(), "NEWAPI_SMTP_TLS_MODE="+mode, "NEWAPI_SMTP_TLS_UNTRUSTED="+fmt.Sprint(!trusted), "SSL_CERT_FILE="+root, "SSL_CERT_DIR="+t.TempDir())
				if output, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("TLS child failed: %v %s", err, output)
				}
				<-done
			})
		}
	}
}

func TestSMTPDeadlineCoversProtocol(t *testing.T) {
	oldSSL, oldStartTLS, oldPort, oldServer := SMTPSSLEnabled, SMTPStartTLSEnabled, SMTPPort, SMTPServer
	t.Cleanup(func() {
		SMTPSSLEnabled, SMTPStartTLSEnabled, SMTPPort, SMTPServer = oldSSL, oldStartTLS, oldPort, oldServer
	})
	for _, stage := range []string{"greeting", "tls", "auth", "mail", "rcpt", "data", "quit"} {
		t.Run(stage, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			SMTPSSLEnabled, SMTPStartTLSEnabled, SMTPPort, SMTPServer = stage == "tls", false, 2525, "localhost"
			done := make(chan struct{})
			go func() {
				defer close(done)
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
				reader := bufio.NewReader(conn)
				if stage == "greeting" || stage == "tls" {
					_, _ = io.Copy(io.Discard, reader)
					return
				}
				_, _ = fmt.Fprint(conn, "220 fixture ESMTP\r\n")
				for {
					line, err := reader.ReadString('\n')
					if err != nil {
						return
					}
					fields := strings.Fields(line)
					if len(fields) == 0 {
						continue
					}
					command := strings.ToLower(fields[0])
					if command == stage {
						_, _ = reader.ReadString('\n')
						return
					}
					if command == "ehlo" && stage == "auth" {
						_, _ = fmt.Fprint(conn, "250-fixture\r\n250 AUTH PLAIN\r\n")
					} else {
						_, _ = fmt.Fprint(conn, "250 fixture\r\n")
					}
				}
			}()
			start := time.Now()
			client, err := newSMTPClient(listener.Addr().String(), 100*time.Millisecond, 150*time.Millisecond)
			if err == nil {
				defer client.Close()
				switch stage {
				case "auth":
					err = client.Auth(smtp.PlainAuth("", "fixture", "synthetic", "localhost"))
				case "mail":
					err = client.Mail("sender@fixture.invalid")
				case "rcpt":
					err = client.Rcpt("receiver@fixture.invalid")
				case "data":
					_, err = client.Data()
				case "quit":
					err = client.Quit()
				}
			}
			if err == nil || time.Since(start) > time.Second {
				t.Fatalf("%s did not fail within budget: %v", stage, err)
			}
			var timeout net.Error
			if !errors.As(err, &timeout) || !timeout.Timeout() {
				t.Fatalf("expected deadline failure, got %v", err)
			}
			if client != nil {
				_ = client.Close()
			}
			<-done
		})
	}
}

func TestSMTPSuccessfulLocalDelivery(t *testing.T) {
	oldSSL, oldStartTLS, oldPort, oldServer := SMTPSSLEnabled, SMTPStartTLSEnabled, SMTPPort, SMTPServer
	t.Cleanup(func() {
		SMTPSSLEnabled, SMTPStartTLSEnabled, SMTPPort, SMTPServer = oldSSL, oldStartTLS, oldPort, oldServer
	})
	SMTPSSLEnabled, SMTPStartTLSEnabled, SMTPPort, SMTPServer = false, false, 2525, "localhost"
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	delivered := make(chan string, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			delivered <- ""
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
		reader := bufio.NewReader(conn)
		_, _ = fmt.Fprint(conn, "220 fixture ESMTP\r\n")
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			if strings.HasPrefix(line, "DATA") {
				_, _ = fmt.Fprint(conn, "354 send data\r\n")
				var body strings.Builder
				for {
					line, err = reader.ReadString('\n')
					if err != nil {
						return
					}
					if line == ".\r\n" {
						break
					}
					body.WriteString(line)
				}
				delivered <- body.String()
			} else if strings.HasPrefix(line, "QUIT") {
				_, _ = fmt.Fprint(conn, "221 bye\r\n")
				return
			}
			_, _ = fmt.Fprint(conn, "250 ok\r\n")
		}
	}()
	client, err := newSMTPClient(listener.Addr().String(), time.Second, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.Mail("sender@fixture.invalid"); err != nil {
		t.Fatal(err)
	}
	if err := client.Rcpt("receiver@fixture.invalid"); err != nil {
		t.Fatal(err)
	}
	w, err := client.Data()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(w, "Subject: fixture\r\n\r\nsynthetic body\r\n"); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := client.Quit(); err != nil {
		t.Fatal(err)
	}
	select {
	case body := <-delivered:
		if !strings.Contains(body, "synthetic body") {
			t.Fatal("message not delivered")
		}
	case <-time.After(time.Second):
		t.Fatal("fixture did not receive message")
	}
}

func TestSMTPSTARTTLSDeadline(t *testing.T) {
	oldSSL, oldStartTLS, oldPort, oldServer := SMTPSSLEnabled, SMTPStartTLSEnabled, SMTPPort, SMTPServer
	t.Cleanup(func() {
		SMTPSSLEnabled, SMTPStartTLSEnabled, SMTPPort, SMTPServer = oldSSL, oldStartTLS, oldPort, oldServer
	})
	SMTPSSLEnabled, SMTPStartTLSEnabled, SMTPPort, SMTPServer = false, true, 2525, "localhost"
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
		reader := bufio.NewReader(conn)
		_, _ = fmt.Fprint(conn, "220 fixture ESMTP\r\n")
		if _, err := reader.ReadString('\n'); err != nil {
			return
		}
		_, _ = fmt.Fprint(conn, "250-fixture\r\n250 STARTTLS\r\n")
		if _, err := reader.ReadString('\n'); err != nil {
			return
		}
		_, _ = fmt.Fprint(conn, "220 begin TLS\r\n")
		_, _ = io.Copy(io.Discard, reader)
	}()
	start := time.Now()
	client, err := newSMTPClient(listener.Addr().String(), 100*time.Millisecond, 150*time.Millisecond)
	if client != nil {
		_ = client.Close()
	}
	var timeout net.Error
	if !errors.As(err, &timeout) || !timeout.Timeout() || time.Since(start) > time.Second {
		t.Fatalf("STARTTLS not bounded: %v", err)
	}
	<-done
}
