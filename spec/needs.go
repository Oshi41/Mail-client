package spec

import (
	"crypto/aes"
	"crypto/cipher"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
)

var (
	storedPassFile = "./key.txt"
)

///////////////////////////////
// Need for stupid spec
//////////////////////////////

// Парсит аргументы, сохраняя шифрованный пароль либо читая его
// Возвращает Mail-адрес, пароль и ошибку
func Parse(args []string) (string, string, error) {
	help := "Usage:\n[Mail] [Key] \n or \n[Mail] [Password] [Key] [Repeat Key]\n Mail - your mail address\nPassword - mail box address\nKey - encryption key for password. From 6 to 32 chars!"

	if len(args) == 2 {
		encrypted, err := ioutil.ReadFile(storedPassFile)
		if err != nil {
			return "", "", err
		}

		pass, err := decrypt(encrypted, args[1])
		if err != nil {
			return "", "", err
		}

		return args[0], pass, nil
	}

	if len(args) == 4 {

		// Обрабатываю ошибку в ключах
		if args[2] != args[3] {
			return "", "", errors.New("Keys do not match")
		}

		encrypted, err := encrypt(args[1], args[2])
		if err != nil {
			return "", "", err
		}

		file, err := os.Create(storedPassFile)
		if err != nil {
			return "", "", err
		}

		_, err = file.Write(encrypted)
		if err != nil {
			return "", "", err
		}

		err = file.Close()
		if err != nil {
			return "", "", err
		}

		log.Println("Pass encrypted")
		return args[0], args[1], nil
	}

	return "", "", errors.New(help)
}

func encrypt(pass, key string) ([]byte, error) {
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
