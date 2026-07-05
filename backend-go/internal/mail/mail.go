// Package mail sends transactional email over SMTP. When SMTP_HOST is empty
// (dev), SendMail logs the message body instead of sending — so local dev works
// without a mail provider.
package mail

import (
	"fmt"
	"log"
	"net/smtp"
)

// SendMail sends a plain/HTML email. If host is empty the message is logged.
func SendMail(host, port, user, pass, from, to, subject, htmlBody string) error {
	if host == "" {
		log.Printf("[mail] SMTP_HOST not set; logging message to %s:\n%s", to, htmlBody)
		return nil
	}
	addr := host + ":" + port
	var auth smtp.Auth
	if user != "" {
		auth = smtp.PlainAuth("", user, pass, host)
	}
	msg := []byte(fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n%s",
		from, to, subject, htmlBody,
	))
	return smtp.SendMail(addr, auth, from, []string{to}, msg)
}
