package outbound

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"time"
)

type svntCryptoEnv struct {
	block cipher.Block
	iv    []byte
	auth  string
}

type svntCryptoConn struct {
	net.Conn
	dec *cipher.StreamReader
	enc *cipher.StreamWriter
}

func newSVNTCryptoEnv(keyData string) (*svntCryptoEnv, error) {
	block, err := aes.NewCipher([]byte(keyData))
	if err != nil {
		return nil, fmt.Errorf("invalid key-data: %w", err)
	}

	iv := make([]byte, aes.BlockSize)
	keyMD5 := md5.Sum([]byte(keyData))
	for i := 0; i < aes.BlockSize; i++ {
		iv[i] = keyMD5[i]
	}

	encrypted := make([]byte, len(svntAuthPlaintext))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(encrypted, []byte(svntAuthPlaintext))
	return &svntCryptoEnv{
		block: block,
		iv:    iv,
		auth:  base64.StdEncoding.EncodeToString(encrypted),
	}, nil
}

func (e *svntCryptoEnv) wrapConn(conn net.Conn) net.Conn {
	return &svntCryptoConn{
		Conn: conn,
		dec: &cipher.StreamReader{
			S: cipher.NewCFBDecrypter(e.block, e.iv),
			R: conn,
		},
		enc: &cipher.StreamWriter{
			S: cipher.NewCFBEncrypter(e.block, e.iv),
			W: conn,
		},
	}
}

func (c *svntCryptoConn) Read(p []byte) (int, error) {
	return c.dec.Read(p)
}

func (c *svntCryptoConn) Write(p []byte) (int, error) {
	return c.enc.Write(p)
}

func (c *svntCryptoConn) Close() error {
	return c.Conn.Close()
}

func (c *svntCryptoConn) LocalAddr() net.Addr {
	return c.Conn.LocalAddr()
}

func (c *svntCryptoConn) RemoteAddr() net.Addr {
	return c.Conn.RemoteAddr()
}

func (c *svntCryptoConn) SetDeadline(t time.Time) error {
	return c.Conn.SetDeadline(t)
}

func (c *svntCryptoConn) SetReadDeadline(t time.Time) error {
	return c.Conn.SetReadDeadline(t)
}

func (c *svntCryptoConn) SetWriteDeadline(t time.Time) error {
	return c.Conn.SetWriteDeadline(t)
}

var _ io.ReadWriteCloser = (*svntCryptoConn)(nil)
