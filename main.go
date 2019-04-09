package main

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/mxk/go-imap/imap"
	"gopkg.in/alecthomas/kingpin.v2"
	"log"
	"net"
	"net/smtp"
	"os"
	"strings"
	"time"
	"net/mail"
	"bytes"
)

var (
	reader = bufio.NewReader(os.Stdin)
	// к какому серверу почты будем подрубаться
	currentMailServer *MailServer

	startApp = kingpin.New("Mail smtpClient, based on console app", "")
	App      = kingpin.New("Mail smtpClient, based on console app", "Supported commands: [send] [get] [exit]")

	// Обязательные аргументы
	Mail = startApp.Arg("addr", "Your mail box address").Required().String()
	Pass = startApp.Arg("pass", "Your password").Required().String()

	Send = App.Command("send", "Send e-mail to chosen addresses")

	Get    = App.Command("get", "Get mail from your mailbox")
	Unread = Get.Flag("unread", "Will get only unread messages").Default("false").Bool()
	Count  = Get.Flag("count", "Amout of loading messages").Default("15").Uint32()

	// Список серверов
	Servers = []MailServer{
		{
			LocalizedName: "Yandex",

			Smtp: "smtp.yandex.ru:465",
			Pop3: "pop.yandex.ru:995",
			Imap: "imap.yandex.ru:993",

			Indexes: []string{
				"yandex.ru",
			},
		},
		{
			LocalizedName: "Mail Ru",

			Smtp: "smtp.mail.ru:465",
			Pop3: "pop.mail.ru:995",
			Imap: "imap.mail.ru:993",

			Indexes: []string{
				"mail.ru",
				"inbox.ru",
				"list.ru",
				"bk.ru",
			},
		},

		// Гугл у нас особенный, его выпиливаем
		//{
		//	LocalizedName:"Google mail",
		//
		//	Smtp:"smtp.gmail.com:465",
		//	Pop3:"pop.gmail.com:995",
		//	Imap:"imap.gmail.com:993",
		//
		//	Indexes: []string{
		//		"gmail.com",
		//	},
		//},
	}
)

func main() {
	// Парсим сначала аргументы
	_, err := startApp.Parse(os.Args[1:])
	if err != nil {
		log.Fatal(err)
	}

	// Ищем сервер
	currentMailServer, err = findServer(*Mail)
	if err != nil {
		log.Fatal(err)
	}

	log.Println(App.Help)

	for {
		line := readLineCarefully()

		command, err := App.Parse(strings.Split(line, " "))
		if err != nil {
			log.Println(err)
			continue
		}

		if line == "exit" {
			return
		}

		switch command {
		case Send.FullCommand():
			sendMail()
			break

		case Get.FullCommand():
			getMessages()
			break
		}
	}
}

///////////////////////////////
// CUSTOM TYPE
///////////////////////////////
type MailServer struct {
	// Список фрагментов адреса почты, идущих после "@"
	// (example@mail.ru -> mail.ru)
	Indexes []string

	// Адрес SMTP сервера
	// Порт обязателен!
	Smtp string

	// Адрес POP3 сервера
	// Порт обязателен!
	Pop3 string

	// Адрес IMAP сервера
	// Порт обязателен!
	Imap string

	// Имя компании
	LocalizedName string
}

type Email struct {
	// Наш адрес
	sender string

	// кому пишем
	to []string

	// Тема письма
	subject string

	// Тело сообщения
	body string
}

// Генерирует тело сообщения
func (email *Email) BuildMessage() []byte {
	enter := "\r\n"

	message := ""
	message += fmt.Sprintf("From: %s%s", email.sender, enter)
	if len(email.to) > 0 {
		message += fmt.Sprintf("To: %s%s", strings.Join(email.to, ";"), enter)
	}

	message += fmt.Sprintf("Subject: %s%s", email.subject, enter)
	message += enter + email.body
	message += enter + "."

	return []byte(message)
}

///////////////////////////////
// HELPING COMMANDS
///////////////////////////////

// Ищет сервер по имени почты
func findServer(addr string) (*MailServer, error) {
	index := strings.Index(addr, "@")
	if index < 0 {
		return nil, errors.New("Address should contains \"@\"")
	}

	userSuffix := addr[index+1:]

	for _, server := range Servers {
		for _, suffix := range server.Indexes {
			if suffix == userSuffix {
				return &server, nil
			}
		}
	}

	return nil, errors.New("Unresolver email address")
}

////////////////////////////////
// COMMAND HANDLERS
////////////////////////////////

// Отправляю почту
func sendMail() {

	conn, host, err := createTLSConn(currentMailServer.Smtp)
	if err != nil {
		log.Println(err)
		return
	}

	// Подключился к SMTP серверу
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		log.Println(err)
		return
	}

	// Авторизовался
	err = client.Auth(smtp.PlainAuth("", *Mail, *Pass, host))
	if err != nil {
		log.Println(err)
		return
	}

	// Формируем тело сообщения
	mail := Email{
		sender: *Mail,
	}

	log.Println("Enter e-mail addresses separated by \";\"")
	mail.to = strings.Split(readLineCarefully(), ";")

	log.Println("Enter subject of e-mail")
	mail.subject = readLineCarefully()

	log.Println("Enter one-line message")
	mail.body = readLineCarefully()

	err = client.Mail(*Mail)
	if err != nil {
		log.Println(err)
		return
	}

	for _, addr := range mail.to {
		err = client.Rcpt(addr)
		if err != nil {
			log.Println(err)
			return
		}
	}

	writer, err := client.Data()
	if err != nil {
		log.Println(err)
		return
	}

	_, err = writer.Write(mail.BuildMessage())
	if err != nil {
		log.Println(err)
		return
	}

	log.Println(client.Quit())
}

// Получаю список сообщений
func getMessages() {

	conn, host, err := createTLSConn(currentMailServer.Imap)
	if err != nil {
		log.Println(err)
		return
	}

	client, err := imap.NewClient(conn, host, time.Second*5)
	if err != nil {
		log.Println(err)
		return
	}

	cmd, err := client.Login(*Mail, *Pass)
	if err != nil {
		log.Println(err)
		return
	}

	cmd, err = cmd.Client().Select("INBOX", false)
	if err != nil {
		log.Println(err)
		return
	}

	// client = cmd.Client()

	// Ограничили кол-во загружаемой почты
	set, _ := imap.NewSeqSet("")
	if client.Mailbox.Messages >= *Count {
		set.AddRange(client.Mailbox.Messages + 1 - *Count, client.Mailbox.Messages)
	} else {
		set.Add("1:*")
	}

	cmd, _ = client.Fetch(set, "RFC822.HEADER")

	for cmd.InProgress() {

		// убираем таймаут
		client.Recv(-1)

		for _, resp := range client.Data {
			if resp.MessageInfo() != nil {

				header := imap.AsBytes(resp.MessageInfo().Attrs["RFC822.HEADER"])

				if msg, _ := mail.ReadMessage(bytes.NewReader(header)); msg != nil {

					fmt.Println("|--", msg.Header.Get("Subject"))
				}
			}
		}

		cmd.Data = nil

		for _, rsp := range client.Data {
			fmt.Println("Server data:", rsp)
		}

		client.Data = nil
	}

}

// Считываю строку до энтера, исключая его
func readLineCarefully() string {
	line, err := reader.ReadString('\n')
	if err != nil {
		log.Println(err)
		return ""
	}

	return line[:len(line)-1]
}

// Создаем защищенное содинение с сервером
func createTLSConn(addr string) (*tls.Conn, string, error) {
	// Выцепил хост из адреса сервера
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, "", err
	}

	// TLS config
	config := &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         host,
	}

	conn, err := tls.Dial("tcp", addr, config)
	return conn, host, err
}
