package io

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"strings"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
)

// DecodeQRFromFile reads an image file and decodes any QR code found in it.
// Returns the decoded text (usually a URL) or an error.
func DecodeQRFromFile(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("无法打开文件: %w", err)
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return "", fmt.Errorf("无法解码图片: %w", err)
	}

	return decodeQRFromImage(img)
}

// decodeQRFromImage decodes a QR code from an image.Image.
func decodeQRFromImage(img image.Image) (string, error) {
	bmp, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		return "", fmt.Errorf("无法创建位图: %w", err)
	}

	qrReader := qrcode.NewQRCodeReader()
	result, err := qrReader.Decode(bmp, nil)
	if err != nil {
		return "", fmt.Errorf("未检测到二维码: %w", err)
	}

	text := strings.TrimSpace(result.GetText())
	if text == "" {
		return "", fmt.Errorf("二维码内容为空")
	}

	return text, nil
}

// IsSurveyURL checks if a decoded QR text looks like a survey URL.
func IsSurveyURL(text string) bool {
	lower := strings.ToLower(text)
	surveyDomains := []string{
		"wjx.cn",
		"wj.qq.com",
		"credamo.com",
	}
	for _, domain := range surveyDomains {
		if strings.Contains(lower, domain) {
			return true
		}
	}
	return false
}

// DecodeSurveyURLFromFile reads a QR code image and extracts a survey URL.
func DecodeSurveyURLFromFile(filePath string) (string, error) {
	text, err := DecodeQRFromFile(filePath)
	if err != nil {
		return "", err
	}
	if !IsSurveyURL(text) {
		return "", fmt.Errorf("二维码内容不是问卷链接: %s", text)
	}
	return text, nil
}
