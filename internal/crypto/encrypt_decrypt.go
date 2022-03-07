package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"golang.org/x/crypto/pbkdf2"
	"strings"
)

func Encrypt(passphrase string, plaintext []byte) (string, error) {
	if passphrase == "" {
		return "", errors.New("passphrase cannot be empty")
	}
	cipherKey, salt, err := deriveCipherKey(passphrase, nil)
	if err != nil {
		return "", fmt.Errorf("error generating cipher key: %w", err)
	}

	cipherBlock, err := aes.NewCipher(cipherKey)
	if err != nil {
		return "", fmt.Errorf("error creating AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(cipherBlock)
	if err != nil {
		return "", fmt.Errorf("error creating GCM cipher")
	}

	nonce := make([]byte, gcm.NonceSize())
	_, err = rand.Read(nonce)
	if err != nil {
		return "", fmt.Errorf("error generating nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)
	return strings.Join([]string{hex.EncodeToString(salt), hex.EncodeToString(nonce), hex.EncodeToString(ciphertext)}, "-"), nil
}

func Decrypt(passphrase string, data string) ([]byte, error) {
	arr := strings.Split(data, "-")
	salt, err := hex.DecodeString(arr[0])
	if err != nil {
		return nil, fmt.Errorf("error decoding hex data: %w", err)
	}

	nonce, err := hex.DecodeString(arr[1])
	if err != nil {
		return nil, fmt.Errorf("error decoding hex data: %w", err)
	}

	ciphertext, err := hex.DecodeString(arr[2])
	if err != nil {
		return nil, fmt.Errorf("error decoding hex data: %w", err)
	}

	key, _, err := deriveCipherKey(passphrase, salt)
	if err != nil {
		return nil, fmt.Errorf("error deriving cipher key: %w", err)
	}

	blockCipher, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("error creating AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(blockCipher)
	if err != nil {
		return nil, fmt.Errorf("error creating GCM cipher: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("error decrypting data (incorrect passphrase?): %w", err)
	}
	return plaintext, nil
}

func deriveCipherKey(passphrase string, salt []byte) ([]byte, []byte, error) {
	if salt == nil {
		salt = make([]byte, 8)
		_, err := rand.Read(salt)
		if err != nil {
			return nil, nil, err
		}
	}
	return pbkdf2.Key([]byte(passphrase), salt, 1000, 32, sha256.New), salt, nil
}
