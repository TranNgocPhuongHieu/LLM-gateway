package service

import (
	"bufio"
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
	"github.com/TranNgocPhuongHieu/LLM-gateway/constant"
	"github.com/TranNgocPhuongHieu/LLM-gateway/relaykit/dto"
)

func TestRetiredNotificationMethodsFailBeforeDelivery(t *testing.T) {
	for _, method := range []string{"webhook", "bark", "gotify", "unknown"} {
		err := NotifyUser(1, "", dto.UserSetting{NotifyType: method}, dto.NewNotify(dto.NotifyTypeQuotaExceed, "fixture", "fixture", nil))
		if err == nil || !strings.Contains(err.Error(), "unsupported notification method") {
			t.Fatalf("%s: %v", method, err)
		}
	}
}

func TestEmailNotificationDelivery(t *testing.T) {
	oldLimit := constant.NotifyLimitCount
	constant.NotifyLimitCount = 10
	t.Cleanup(func() { constant.NotifyLimitCount = oldLimit })
	oldServer, oldPort, oldFrom := common.SMTPServer, common.SMTPPort, common.SMTPFrom
	oldAccount, oldToken := common.SMTPAccount, common.SMTPToken
	oldSSL, oldTLS, oldRedis := common.SMTPSSLEnabled, common.SMTPStartTLSEnabled, common.RedisEnabled
	common.SMTPServer, common.SMTPFrom = "127.0.0.1", "sender@example.invalid"
	common.SMTPAccount, common.SMTPToken = "", ""
	common.SMTPSSLEnabled, common.SMTPStartTLSEnabled, common.RedisEnabled = false, false, false
	t.Cleanup(func() {
		common.SMTPServer, common.SMTPPort, common.SMTPFrom = oldServer, oldPort, oldFrom
		common.SMTPAccount, common.SMTPToken = oldAccount, oldToken
		common.SMTPSSLEnabled, common.SMTPStartTLSEnabled, common.RedisEnabled = oldSSL, oldTLS, oldRedis
	})
	for i, method := range []string{"", "email"} {
		t.Run(fmt.Sprintf("email-%d", i), func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			_, port, _ := net.SplitHostPort(listener.Addr().String())
			common.SMTPPort, _ = strconv.Atoi(port)
			received := make(chan string, 1)
			go func() {
				conn, err := listener.Accept()
				if err != nil {
					received <- err.Error()
					return
				}
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
				reader := bufio.NewReader(conn)
				fmt.Fprint(conn, "220 fixture SMTP\r\n")
				var transcript strings.Builder
				for {
					line, err := reader.ReadString('\n')
					if err != nil {
						received <- transcript.String()
						return
					}
					transcript.WriteString(line)
					switch {
					case strings.HasPrefix(line, "DATA"):
						fmt.Fprint(conn, "354 continue\r\n")
						for {
							line, err = reader.ReadString('\n')
							if err != nil || line == ".\r\n" {
								break
							}
							transcript.WriteString(line)
						}
						fmt.Fprint(conn, "250 accepted\r\n")
					case strings.HasPrefix(line, "QUIT"):
						fmt.Fprint(conn, "221 bye\r\n")
						received <- transcript.String()
						return
					default:
						fmt.Fprint(conn, "250 OK\r\n")
					}
				}
			}()
			setting := dto.UserSetting{NotifyType: method}
			want := "account@example.invalid"
			if method == "email" {
				setting.NotificationEmail = "override@example.invalid"
				want = setting.NotificationEmail
			}
			err = NotifyUser(8700+i, "account@example.invalid", setting, dto.NewNotify(dto.NotifyTypeQuotaExceed, "fixture", "Remaining {{value}}", []any{123}))
			if err != nil {
				t.Fatal(err)
			}
			select {
			case transcript := <-received:
				if !strings.Contains(transcript, "RCPT TO:<"+want+">") || !strings.Contains(transcript, "Remaining 123") {
					t.Fatalf("unexpected delivery: %s", transcript)
				}
			case <-time.After(6 * time.Second):
				t.Fatal("SMTP fixture timed out")
			}
		})
	}
}
