package main

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-message/mail"
	"gopkg.in/alecthomas/kingpin.v2"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/smtp"
	"os"
	"strings"
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
	Count  = Get.Arg("count", "Amout of loading messages").Default("10").Uint32()

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
	smtpClient, err := smtp.NewClient(conn, host)
	if err != nil {
		log.Println(err)
		return
	}

	// Авторизовался
	err = smtpClient.Auth(smtp.PlainAuth("", *Mail, *Pass, host))
	if err != nil {
		log.Println(err)
		return
	}

	// Формируем тело сообщения
	email := Email{
		sender: *Mail,
	}

	log.Println("Enter e-email addresses separated by \";\"")
	email.to = strings.Split(readLineCarefully(), ";")

	log.Println("Enter subject of e-email")
	email.subject = readLineCarefully()

	log.Println("Enter one-line message")
	email.body = readLineCarefully()

	err = smtpClient.Mail(*Mail)
	if err != nil {
		log.Println(err)
		return
	}

	for _, addr := range email.to {
		err = smtpClient.Rcpt(addr)
		if err != nil {
			log.Println(err)
			return
		}
	}

	writer, err := smtpClient.Data()
	if err != nil {
		log.Println(err)
		return
	}

	_, err = writer.Write(email.BuildMessage())
	if err != nil {
		log.Println(err)
		return
	}

	log.Println(smtpClient.Quit())
}

// Получаю список сообщений
func getMessages() {
	// Создал TLS соединение
	connection, _, err := createTLSConn(currentMailServer.Imap)
	if err != nil {
		log.Println(err)
		return
	}

	// На основе соединения создал IMAP клиента
	imapClient, err := client.New(connection)
	if err != nil {
		log.Println(err)
		return
	}

	// Залогинился
	err = imapClient.Login(*Mail, *Pass)
	if err != nil {
		log.Println(err)
		return
	}

	// Получил списки папок на моем аккаунте
	mailboxes := make(chan *imap.MailboxInfo, 10)
	done := make(chan error, 1)
	go func() {
		done <- imapClient.List("", "*", mailboxes)
	}()

	// Жду окончания операции
	if err := <-done; err != nil {
		log.Println(err)
		return
	}

	// Нас интересуют входящие
	inbox, err := imapClient.Select("INBOX", false)
	if err != nil {
		log.Println(err)
		return
	}

	// Создаём критерий запросы
	criteria := imap.NewSearchCriteria()

	if *Unread {
		criteria.WithoutFlags = []string{"\\Seen"}
	}

	// Получаю последние эмейлы, по указанному кол-ву

	from := uint32(1)
	to := inbox.Messages
	if to >= *Count {
		from = inbox.Messages - *Count
	}

	set := new(imap.SeqSet)
	set.AddRange(from, to)
	criteria.SeqNum = set

	// Получаю уникальные UUID найденых писем
	uids, err := imapClient.Search(criteria)
	if err != nil {
		log.Println(err)
	}

	// Создаю список каналов для каждого сообщения
	messages := make(chan *imap.Message, len(uids))
	done = make(chan error, 1)

	section := &imap.BodySectionName{}
	items := []imap.FetchItem{imap.FetchEnvelope, imap.FetchFlags, imap.FetchInternalDate, section.FetchItem()}
	seqset := new(imap.SeqSet)
	seqset.AddNum(uids...)

	go func() {
		done <- imapClient.Fetch(seqset, items, messages)
	}()

	// Ожидаю окончания загрузки сообщений
	err = <-done
	if err != nil {
		log.Println(err)
		return
	}

	for msg := range messages {
		printMessage(msg, section)
	}
}

///////////////////////////////
// HELPING METHODS
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

func printMessage(message *imap.Message, section *imap.BodySectionName) {
	if message == nil {
		log.Println("Message is nil")
		return
	}

	body := message.GetBody(section)
	if body == nil {
		log.Println("Server didn't return message")
		return
	}

	reader, err := mail.CreateReader(body)
	if err != nil {
		log.Println(err)
		return
	}

	// Print some info about the message
	header := reader.Header
	if date, err := header.Date(); err == nil {
		fmt.Println("Date:", date)
	}
	if from, err := header.AddressList("From"); err == nil {
		fmt.Println("From:", from)
	}
	if to, err := header.AddressList("To"); err == nil {
		fmt.Println("To:", to)
	}
	if subject, err := header.Subject(); err == nil && subject != "" {
		fmt.Println("Subject:", subject)
	}

	for {
		part, err := reader.NextPart()
		switch err {
		// Продолжаем читать сообщение
		case nil:
			switch h := part.Header.(type) {
			case *mail.InlineHeader:
				// This is the message's text (can be plain-text or HTML)
				b, _ := ioutil.ReadAll(part.Body)
				fmt.Println(string(b))
			case *mail.AttachmentHeader:
				// This is an attachment
				filename, _ := h.Filename()
				fmt.Println(filename)
			}

			break

		// Прочитали всё сообщений
		case io.EOF:
			return

		// Наткнулись на ошибку
		default:
			log.Println(err)
			return
		}
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
