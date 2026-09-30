package axe

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"
)

const MaxImageBytes = 32 * 1024 * 1024

func Attach(source string) (Image, error) {
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		return Image{URL: source}, nil
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return Image{}, fmt.Errorf("read image %s: %w", source, err)
	}
	return AttachData(source, data)
}

func AttachData(source string, data []byte) (Image, error) {
	if len(data) > MaxImageBytes {
		return Image{}, fmt.Errorf(
			"image %s is %d MiB; the limit is %d MiB",
			source, len(data)/(1024*1024), MaxImageBytes/(1024*1024),
		)
	}
	url := fmt.Sprintf("data:%s;base64,%s", imageMime(data, source), base64.StdEncoding.EncodeToString(data))
	return Image{Path: source, URL: url}, nil
}

func AttachBytes(source string, data []byte) (Image, bool) {
	head := data
	if len(head) > 16 {
		head = head[:16]
	}
	if sniffImage(head) == "" {
		return Image{}, false
	}
	image, err := AttachData(source, data)
	return image, err == nil
}

func AttachIfImage(path string) (Image, bool) {
	file, err := os.Open(path)
	if err != nil {
		return Image{}, false
	}
	defer file.Close()
	head := make([]byte, 16)
	read, err := file.Read(head)
	if err != nil && read == 0 {
		return Image{}, false
	}
	if sniffImage(head[:read]) == "" {
		return Image{}, false
	}
	image, err := Attach(path)
	return image, err == nil
}

func ImageLabel(image Image) string {
	source := image.Path
	if source == "" {
		source = image.URL
	}
	runes := []rune(source)
	if len(runes) <= 60 {
		return source
	}
	return string(runes[:59]) + "…"
}

func IsImagePath(text string) bool {
	if text == "" || strings.Contains(text, "\n") {
		return false
	}
	lower := strings.ToLower(text)
	dot := strings.LastIndex(lower, ".")
	if dot < 0 {
		return false
	}
	switch lower[dot+1:] {
	case "png", "jpg", "jpeg", "gif", "webp":
	default:
		return false
	}
	info, err := os.Stat(text)
	return err == nil && info.Mode().IsRegular()
}

func imageMime(data []byte, path string) string {
	if mime := sniffImage(data); mime != "" {
		return mime
	}
	return mimeByExtension(path)
}

func sniffImage(data []byte) string {
	if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return "image/webp"
	}
	if len(data) >= 4 && data[0] == 0x89 && data[1] == 'P' && data[2] == 'N' && data[3] == 'G' {
		return "image/png"
	}
	if len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff {
		return "image/jpeg"
	}
	if len(data) >= 4 && string(data[:4]) == "GIF8" {
		return "image/gif"
	}
	return ""
}

func mimeByExtension(path string) string {
	dot := strings.LastIndex(path, ".")
	if dot < 0 {
		return "image/png"
	}
	switch strings.ToLower(path[dot+1:]) {
	case "jpg", "jpeg":
		return "image/jpeg"
	case "gif":
		return "image/gif"
	case "webp":
		return "image/webp"
	default:
		return "image/png"
	}
}
