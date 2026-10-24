package gohl

import (
	"archive/zip"
	"bytes"
	"embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

//go:embed resources.zip
var resourcesZip embed.FS

var (
	resourcesDir       string
	resourcesExtracted bool
)

func extractResources() {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		appData = os.TempDir()
	}

	resourcesDir = filepath.Join(appData, "gohl")
	os.MkdirAll(resourcesDir, 0755)

	markerFile := filepath.Join(resourcesDir, ".extracted")
	if _, err := os.Stat(markerFile); err == nil {
		resourcesExtracted = true
		return
	}

	data, err := resourcesZip.ReadFile("resources.zip")
	if err != nil {
		fmt.Printf("读取嵌入的 resources.zip 失败: %v\n", err)
		return
	}

	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		fmt.Printf("解压 resources.zip 失败: %v\n", err)
		return
	}

	for _, file := range reader.File {
		dstPath := filepath.Join(resourcesDir, file.Name)

		if file.FileInfo().IsDir() {
			os.MkdirAll(dstPath, file.Mode())
			continue
		}

		os.MkdirAll(filepath.Dir(dstPath), 0755)

		srcFile, err := file.Open()
		if err != nil {
			fmt.Printf("打开压缩文件 %s 失败: %v\n", file.Name, err)
			continue
		}

		dstFile, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
		if err != nil {
			srcFile.Close()
			fmt.Printf("创建文件 %s 失败: %v\n", dstPath, err)
			continue
		}

		_, err = io.Copy(dstFile, srcFile)
		srcFile.Close()
		dstFile.Close()

		if err != nil {
			fmt.Printf("写入文件 %s 失败: %v\n", dstPath, err)
			continue
		}

		fmt.Printf("释放资源: %s\n", dstPath)
	}

	marker, err := os.Create(markerFile)
	if err == nil {
		marker.Close()
	}

	resourcesExtracted = true
	fmt.Printf("资源已解压到: %s\n", resourcesDir)
}

func GetResourcesDir() string {
	return resourcesDir
}
