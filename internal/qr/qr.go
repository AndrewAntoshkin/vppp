package qr

import (
	"encoding/base64"
	"fmt"

	"github.com/skip2/go-qrcode"
)

func GeneratePNG(content string, size int) ([]byte, error) {
	png, err := qrcode.Encode(content, qrcode.Medium, size)
	if err != nil {
		return nil, fmt.Errorf("generate qr: %w", err)
	}
	return png, nil
}

func GenerateBase64(content string, size int) (string, error) {
	png, err := GeneratePNG(content, size)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(png), nil
}

func GenerateTerminal(content string) (string, error) {
	q, err := qrcode.New(content, qrcode.Medium)
	if err != nil {
		return "", fmt.Errorf("generate qr: %w", err)
	}
	return q.ToSmallString(false), nil
}
