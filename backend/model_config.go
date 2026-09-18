package main

import (
	"errors"
	"strconv"
	"strings"
)

// Image settings are validated before reserving quota or contacting the provider.
func imageSettings() (model, size string, err error) {
	model, size = env("BIGMODEL_IMAGE_MODEL", "glm-image"), env("BIGMODEL_IMAGE_SIZE", "1152x1536")
	parts := strings.Split(size, "x")
	if len(parts) != 2 {
		return "", "", errors.New("图片尺寸格式应为宽x高")
	}
	w, e1 := strconv.Atoi(parts[0])
	h, e2 := strconv.Atoi(parts[1])
	min, step, pixels := 512, 16, 1<<21
	switch model {
	case "glm-image":
		min, step, pixels = 1024, 32, 1<<22
	case "cogview-4", "cogview-4-250304", "cogview-3-flash":
	default:
		return "", "", errors.New("尚未适配该图片模型")
	}
	if e1 != nil || e2 != nil || w < min || h < min || w > 2048 || h > 2048 || w%step != 0 || h%step != 0 || w*h > pixels {
		return "", "", errors.New("图片尺寸不符合模型约束")
	}
	return model, size, nil
}
