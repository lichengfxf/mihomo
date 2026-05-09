package outbound

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"encoding/base64"
	"fmt"
)

func generateSVNTAuth(keyData string) (string, error) {
	block, err := aes.NewCipher([]byte(keyData))
	if err != nil {
		return "", fmt.Errorf("invalid key-data: %w", err)
	}

	iv := make([]byte, aes.BlockSize)
	keyMD5 := md5.Sum([]byte(keyData))
	for i := 0; i < aes.BlockSize; i++ {
		iv[i] = keyMD5[i]
	}

	encrypted := make([]byte, len(svntAuthPlaintext))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(encrypted, []byte(svntAuthPlaintext))
	return base64.StdEncoding.EncodeToString(encrypted), nil
}
