package spec

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
)

var (
	storedPassFile = "./config.json"
)

//////////////////////////////
// JSON type
/////////////////////////////
type config struct {
	Mail string `json:"mail"`
	Pass []byte `json:"pass"`
}

func readEndDecrypt(key string) (string, string, error) {
	raw, err := ioutil.ReadFile(storedPassFile)
	if err != nil {
		return "", "", err
	}

	cfg := config{}
	// Обязательно передаём указатель, требуется по спеке
	err = json.Unmarshal(raw, &cfg)
	if err != nil {
		return "", "", err
	}

	pass, err := decrypt(cfg.Pass, key)
	if err != nil {
		return "", "", err
	}

	return cfg.Mail, pass, nil
}

func encryptAndWrite(mail, pass, key string) error {
	file, err := os.Create(storedPassFile)
	if err != nil {
		return err
	}

	defer file.Close()

	encrypted, err := encrypt([]byte(pass), key)
	if err != nil {
		return err
	}

	cfg := config{
		Mail: mail,
		Pass: encrypted,
	}

	toWrite, err := json.Marshal(cfg)
	if err != nil {
		return err
	}

	_, err = file.Write(toWrite)
	if err != nil {
		return err
	}

	log.Println("Config saved")
	return nil
}

///////////////////////////////
// Need for stupid spec
//////////////////////////////

// Парсит аргументы, сохраняя шифрованный пароль либо читая его
// Возвращает Mail-адрес, пароль и ошибку
func Parse(args []string) (string, string, error) {
	help := "Usage:\n[Key] \n" +
		" or " +
		"\n[Mail] [Password] [Key] [Repeat Key]\n" +
		"Mail - your mail address\n" +
		"Password - mail box address\n" +
		"Key - encryption key for password. From 6 to 32 chars!"

	if len(args) == 1 {
		return readEndDecrypt(args[0])
	}

	if len(args) == 4 {

		// Обрабатываю ошибку в ключах
		if args[2] != args[3] {
			return "", "", errors.New("Keys do not match")
		}

		err := encryptAndWrite(args[0], args[1], args[2])
		if err != nil {
			return "", "", err
		}

		return args[0], args[1], nil
	}

	return "", "", errors.New(help)
}

func encrypt(pass []byte, key string) ([]byte, error) {
	if len(key) < 6 || len(key) > 32 {
		return nil, errors.New("Wrong key length")
	}

	// Фиксирую длину пароля
	fixedKey := []byte(fmt.Sprintf("%32.32s", key))
	// соль - 12 символов
	nonce := []byte(fmt.Sprintf("%12.12s", key))

	block, err := aes.NewCipher(fixedKey)
	if err != nil {
		return nil, err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	return aesgcm.Seal(nil, nonce, []byte(pass), nil), nil
}

func decrypt(pass []byte, key string) (string, error) {

	// Фиксирую длину пароля
	fixedKey := []byte(fmt.Sprintf("%32.32s", key))
	// соль - 12 символов
	nonce := []byte(fmt.Sprintf("%12.12s", key))

	block, err := aes.NewCipher(fixedKey)
	if err != nil {
		return "", err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	decoded, err := aesgcm.Open(nil, nonce, pass, nil)
	if err != nil {
		return "", err
	}

	return string(decoded), nil
}
