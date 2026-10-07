package common

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"slices"
	"strconv"
	"strings"
	"time"
)

type smtpDeliveryProfile struct {
	host        string
	port        int
	from        string
	account     string
	secret      string
	implicitTLS bool
	startTLS    bool
	skipVerify  bool
}

func snapshotSMTPProfile() smtpDeliveryProfile {
	from := SMTPFrom
	if from == "" {
		from = SMTPAccount
	}
	return smtpDeliveryProfile{
		host: SMTPServer, port: SMTPPort, from: from, account: SMTPAccount, secret: SMTPToken,
		implicitTLS: SMTPSSLEnabled || SMTPPort == 465 && !SMTPStartTLSEnabled,
		startTLS:    SMTPStartTLSEnabled, skipVerify: SMTPInsecureSkipVerify,
	}
}

func generateMessageID() (string, error) {
	return smtpMessageID(snapshotSMTPProfile().from)
}
func smtpMessageID(sender string) (string, error) {
	address, err := mail.ParseAddress(sender)
	if err != nil {
		return "", errors.New("SMTP 发件人地址无效。")
	}
	_, domain, found := strings.Cut(address.Address, "@")
	if !found || domain == "" {
		return "", errors.New("SMTP 发件人地址无效。")
	}
	return fmt.Sprintf("<%d.%s@%s>", time.Now().UnixNano(), GetRandomString(12), domain), nil
}
func shouldUseSMTPLoginAuth() bool {
	return SMTPForceAuthLogin || isOutlookServer(SMTPAccount) || slices.Contains(EmailLoginAuthServerList, SMTPServer)
}
func getSMTPAuth() smtp.Auth       { return AutoSMTPAuth(SMTPAccount, SMTPToken) }
func shouldAuthenticateSMTP() bool { return SMTPAccount != "" && SMTPToken != "" }
func smtpTLSConfig() *tls.Config {
	profile := snapshotSMTPProfile()
	return &tls.Config{ServerName: profile.host, InsecureSkipVerify: profile.skipVerify, MinVersion: tls.VersionTLS12}
}

func dialSMTPDelivery(ctx context.Context, profile smtpDeliveryProfile) (*smtp.Client, net.Conn, error) {
	address := net.JoinHostPort(profile.host, strconv.Itoa(profile.port))
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	socket, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, nil, err
	}
	connected := socket
	stopCancellation := context.AfterFunc(ctx, func() { _ = connected.Close() })
	defer stopCancellation()
	if deadline, ok := ctx.Deadline(); ok {
		_ = socket.SetDeadline(deadline)
	}
	if profile.implicitTLS {
		secure := tls.Client(socket, &tls.Config{ServerName: profile.host, InsecureSkipVerify: profile.skipVerify, MinVersion: tls.VersionTLS12})
		if err := secure.HandshakeContext(ctx); err != nil {
			_ = socket.Close()
			return nil, nil, err
		}
		socket = secure
	}
	client, err := smtp.NewClient(socket, profile.host)
	if err != nil {
		_ = socket.Close()
		return nil, nil, err
	}
	if profile.startTLS && !profile.implicitTLS {
		if supported, _ := client.Extension("STARTTLS"); !supported {
			_ = client.Close()
			return nil, nil, errors.New("SMTP server does not support STARTTLS")
		}
		if err := client.StartTLS(&tls.Config{ServerName: profile.host, InsecureSkipVerify: profile.skipVerify, MinVersion: tls.VersionTLS12}); err != nil {
			_ = client.Close()
			return nil, nil, err
		}
	}
	return client, socket, nil
}

func newSMTPClient(address string) (*smtp.Client, error) {
	profile := snapshotSMTPProfile()
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	number, err := strconv.Atoi(port)
	if err != nil {
		return nil, err
	}
	profile.host, profile.port = host, number
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, _, err := dialSMTPDelivery(ctx, profile)
	return client, err
}

func smtpRecipients(receiver string) ([]string, error) {
	var result []string
	for _, part := range strings.Split(receiver, ";") {
		if strings.TrimSpace(part) == "" {
			continue
		}
		address, err := mail.ParseAddress(strings.TrimSpace(part))
		if err != nil || strings.ContainsAny(address.Address, "\r\n") {
			return nil, errors.New("SMTP 收件人地址无效。")
		}
		result = append(result, address.Address)
	}
	if len(result) == 0 {
		return nil, errors.New("SMTP 收件人地址不能为空。")
	}
	return result, nil
}

func SendEmail(subject, receiver, content string) error {
	return SendEmailContext(context.Background(), subject, receiver, content)
}

func SendEmailContext(parent context.Context, subject, receiver, content string) error {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	profile := snapshotSMTPProfile()
	if profile.host == "" || profile.port < 1 || profile.port > 65535 {
		return errors.New("SMTP 服务器尚未正确配置。")
	}
	from, err := mail.ParseAddress(profile.from)
	if err != nil {
		return errors.New("SMTP 发件人地址无效。")
	}
	recipients, err := smtpRecipients(receiver)
	if err != nil {
		return err
	}
	id, err := smtpMessageID(from.Address)
	if err != nil {
		return err
	}
	header := fmt.Sprintf("To: %s\r\nFrom: %s\r\nSubject: =?UTF-8?B?%s?=\r\nDate: %s\r\nMessage-ID: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n%s\r\n",
		strings.Join(recipients, ", "), (&mail.Address{Name: "further-api", Address: from.Address}).String(),
		base64.StdEncoding.EncodeToString([]byte(subject)), time.Now().Format(time.RFC1123Z), id, content)
	client, socket, err := dialSMTPDelivery(ctx, profile)
	if err != nil {
		return err
	}
	defer client.Close()
	stop := context.AfterFunc(ctx, func() { _ = socket.Close() })
	defer stop()
	if profile.account != "" && profile.secret != "" {
		if err := client.Auth(AutoSMTPAuth(profile.account, profile.secret)); err != nil {
			return err
		}
	}
	if err := client.Mail(from.Address); err != nil {
		return err
	}
	for _, recipient := range recipients {
		if err := client.Rcpt(recipient); err != nil {
			return err
		}
	}
	body, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := body.Write([]byte(header)); err != nil {
		_ = body.Close()
		return err
	}
	if err := body.Close(); err != nil {
		return err
	}
	// DATA acknowledgement has committed delivery. QUIT failures must not
	// report a delivered message as failed and trigger duplicate notifications.
	_ = client.Quit()
	return nil
}
