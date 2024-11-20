// Super simple file list builder.
package main

import (
	"archive/zip"
	"bufio"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cespare/xxhash"
	"github.com/go-yaml/yaml"
)

// Config represents the configuration file
type Config struct {
	Client         string   `yaml:"client,omitempty"`
	DownloadPrefix string   `yaml:"downloadprefix,omitempty"`
	Manifest       Manifest `yaml:"manifest,omitempty"`
}

// FileList represents a file list
type FileList struct {
	Version        string      `yaml:"version,omitempty"`
	Deletes        []FileEntry `yaml:"deletes,omitempty"`
	DownloadPrefix string      `yaml:"downloadprefix,omitempty"`
	Downloads      []FileEntry `yaml:"downloads,omitempty"`
	Unpacks        []FileEntry `yaml:"unpacks,omitempty"`
}

type Manifest struct {
	ShortName      string            `json:"shortName" yaml:"shortName"`
	LongName       string            `json:"longName" yaml:"longName"`
	Website        string            `json:"website" yaml:"website"`
	Description    string            `json:"description" yaml:"description"`
	Hosts          []string          `json:"hosts" yaml:"hosts"`
	CustomFilesURL string            `json:"customFilesUrl" yaml:"customFilesUrl"`
	FilesURLPrefix string            `json:"filesUrlPrefix" yaml:"filesUrlPrefix"`
	Version        string            `json:"version" yaml:"version"`
	Required       []string          `json:"required" yaml:"required"`
	Files          map[string]string `json:"files" yaml:"files"`
}

// FileEntry represents a file entry
type FileEntry struct {
	Name string `yaml:"name,omitempty"`
	Md5  string `yaml:"md5,omitempty"`
	Date string `yaml:"date,omitempty"`
	Size int64  `yaml:"size,omitempty"`
}

var (
	// Version is exported during build
	Version    string
	ignoreList []FileEntry
	fileList   FileList
	manifest   Manifest
	patchFile  *zip.Writer
)

func main() {
	var err error
	var out []byte
	fmt.Printf("filelistbuilder v%s\n", Version)

	manifest.Files = make(map[string]string)
	config := Config{}

	fmt.Printf("Number of args: %d\n", len(os.Args))
	if len(os.Args) == 4 {
		config.Client = os.Args[1]
		fmt.Println("passed argument 1 client:", config.Client)
		config.DownloadPrefix = os.Args[2]
		fmt.Println("passed argument 2 downloadprefix:", config.DownloadPrefix)

	} else if len(os.Args) == 13 {
		config.Client = os.Args[1]
		fmt.Println("passed argument 1 client:", config.Client)
		config.DownloadPrefix = os.Args[2]
		fmt.Println("passed argument 2 downloadprefix:", config.DownloadPrefix)
		//exe path is 3
		manifest.ShortName = os.Args[4]
		fmt.Println("passed argument 4 shortname:", manifest.ShortName)
		manifest.LongName = os.Args[5]
		fmt.Println("passed argument 5 longname:", manifest.LongName)
		manifest.CustomFilesURL = os.Args[6]
		fmt.Println("passed argument 6 customfilesurl:", manifest.CustomFilesURL)
		manifest.FilesURLPrefix = os.Args[7]
		fmt.Println("passed argument 7 filesurlprefix:", manifest.FilesURLPrefix)
		manifest.Version = os.Args[8]
		fmt.Println("passed argument 8 version:", manifest.Version)
		manifest.Website = os.Args[9]
		fmt.Println("passed argument 9 website:", manifest.Website)
		manifest.Description = os.Args[10]
		fmt.Println("passed argument 10 description:", manifest.Description)
		host := os.Args[11]
		fmt.Println("passed argument 11 host:", host)
		manifest.Hosts = append(manifest.Hosts, host)
		required := os.Args[12]
		fmt.Println("passed argument 12 required:", required)
		manifest.Required = append(manifest.Required, required)
	} else {
		inFile, err := os.ReadFile("filelistbuilder.yml")
		if err != nil {
			log.Fatal("Failed to parse filelistbuilder.yml:", err.Error())
		}

		err = yaml.Unmarshal(inFile, &config)
		if err != nil {
			log.Fatal("Failed to unmarshal filelistbuilder.yml:", err.Error())
		}

		manifest = config.Manifest
		manifest.Files = make(map[string]string)
	}

	h := md5.New()
	if len(os.Args) == 3 {
		exePath := os.Args[3]
		fmt.Println("passed argument 3 exePath:", exePath)
		var md5 string
		md5, err = getMd5(exePath)
		if err != nil {
			fmt.Println("ignoring error exePath getmd5:", err)
		}
		err = os.WriteFile("eqemupatcher-hash.txt", []byte(strings.ToUpper(md5)), os.ModePerm)
		if err != nil {
			fmt.Println("ignoring error exePath:", err)
		}
	}

	if len(config.Client) < 1 {
		log.Fatal("client not set in filelistbuilder.yml or args")
	}

	if len(config.DownloadPrefix) < 1 {
		log.Fatal("downloadprefix not set in filelistbuilder.yml or args")
	}

	fileList.DownloadPrefix = config.DownloadPrefix

	generateIgnores("ignore.txt")

	err = filepath.Walk(".", visit)
	if err != nil {
		log.Fatal("Error filepath", err.Error())
	}

	io.WriteString(h, fmt.Sprintf("%d", time.Now().Nanosecond()))
	for _, d := range fileList.Downloads {
		io.WriteString(h, d.Name)
	}

	fileList.Version = fmt.Sprintf("%s%x", time.Now().Format("20060102"), h.Sum(nil))

	out, err = yaml.Marshal(&fileList)
	if err != nil {
		log.Fatal("Error marshalling:", err.Error())
	}
	if len(fileList.Downloads) == 0 {
		log.Fatal("No files found in directory")
	}

	err = os.WriteFile("filelist_"+config.Client+".yml", out, 0644)
	if err != nil {
		log.Fatal("Failed write: ", err)
	}

	out, err = json.MarshalIndent(&manifest, "", "    ")
	if err != nil {
		log.Fatal("Error marshalling manifest:", err.Error())
	}

	err = os.WriteFile("manifest.json", out, 0644)
	if err != nil {
		log.Fatal("Failed manifest.json write: ", err)
	}

	//Now let's make patch zip.
	createPatch()

	log.Println("Wrote filelist_"+config.Client+".yml and patch.zip with", len(fileList.Downloads), "files inside.")
	log.Println("Wrote manifest.json with", len(manifest.Files), "files inside.")
}

func createPatch() {
	var err error
	var f io.Writer
	var buf *os.File

	if buf, err = os.Create("patch.zip"); err != nil {
		log.Fatal("Failed to create patch.zip", err.Error())
	}

	patchFile = zip.NewWriter(buf)

	for _, download := range fileList.Downloads {
		var in io.Reader
		//fmt.Println("Adding", download.Name)
		if f, err = patchFile.Create(download.Name); err != nil {
			log.Fatal("Failed to create", download.Name, "inside patch:", err.Error())
		}

		if in, err = os.Open(download.Name); err != nil {
			log.Fatal("Failed to open", download.Name, "inside patch:", err.Error())
		}

		if _, err = io.Copy(f, in); err != nil {
			log.Fatal("Failed to copy", download.Name, "inside patch:", err.Error())
		}
	}

	//Now let's create a README.txt
	readme := "Extract the contents of patch.zip to your root EQ directory.\r\n"
	if len(fileList.Deletes) > 0 {
		readme += "Also delete the following files:\r\n"
		for _, del := range fileList.Deletes {
			readme += del.Name + "\r\n"
		}
	}
	if f, err = patchFile.Create("README.txt"); err != nil {
		log.Fatal("Failed to create README.txt inside patch:", err.Error())
	}

	f.Write([]byte(readme))

	if err = patchFile.Close(); err != nil {
		log.Fatal("Error while closing patchfile", err.Error())
	}

}

func visit(path string, f os.FileInfo, err error) error {
	if err != nil {
		return err
	}
	if strings.Contains(path, "eqemupatcher.exe") ||
		strings.Contains(path, "README.md") ||
		strings.Contains(path, ".gitignore") ||
		strings.Contains(path, ".DS_Store") ||
		strings.Contains(path, "filelistbuilder") ||
		strings.Contains(path, "filelist") ||
		strings.Contains(path, "ignore.txt") ||
		strings.Contains(path, "-hash.txt") ||
		path == "patch.zip" {
		return nil
	}

	if !f.IsDir() {

		for _, entry := range ignoreList {
			if path == entry.Name { //ignored file
				return nil
			}
		}
		//found a delete entry list
		if path == "delete.txt" {
			err = generateDeletes(path)
			if err != nil {
				fmt.Println("ignored generateDeletes:", err)
			}
			//Don't conntinue.
			return nil
		}

		download := FileEntry{
			Size: f.Size(),
			Name: path,
			Date: f.ModTime().Format("20060102"),
		}
		var md5Val string
		if md5Val, err = getMd5(path); err != nil {
			log.Fatal("Failed to md5", path, err.Error())
		}
		download.Md5 = md5Val

		fileList.Downloads = append(fileList.Downloads, download)

		xxh3Val, err := getXXH3(path)
		if err != nil {
			log.Fatal("Failed to xxh3", path, err.Error())
		}

		manifest.Files[strings.ReplaceAll(path, "\\", "/")] = xxh3Val
	}
	return nil
}

func getMd5(path string) (value string, err error) {

	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	h := md5.New()
	_, err = io.Copy(h, f)
	if err != nil {
		return
	}
	value = fmt.Sprintf("%x", h.Sum(nil))
	return
}

func getXXH3(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", filePath, err)
	}
	defer file.Close()

	hasher := xxhash.New()

	buffer := make([]byte, 4096)
	for {
		bytesRead, err := file.Read(buffer)
		if err != nil && err != io.EOF {
			return "", err
		}
		if bytesRead == 0 {
			break
		}

		_, hashErr := hasher.Write(buffer[:bytesRead])
		if hashErr != nil {
			return "", hashErr
		}
	}

	return fmt.Sprintf("%016X", hasher.Sum64()), nil
}

func generateIgnores(path string) (err error) {

	//if ignore doesn't exist, no worries.
	if _, err = os.Stat(path); os.IsNotExist(err) {
		err = nil
		return
	}

	file, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		data := scanner.Text()
		if len(data) == 0 {
			continue
		}
		if strings.Contains(data, "#") { //Strip comments
			data = data[0:strings.Index(data, "#")]
		}
		if len(strings.TrimSpace(data)) < 1 { //skip empty lines
			continue
		}

		entry := FileEntry{
			Name: data,
		}
		ignoreList = append(ignoreList, entry)
	}

	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}
	return
}

func generateDeletes(path string) (err error) {
	//if delete doesn't exist, no worries.
	if _, err = os.Stat(path); os.IsNotExist(err) {
		err = nil
		return
	}

	file, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		data := scanner.Text()
		if len(data) == 0 {
			continue
		}
		if strings.Contains(data, "#") { //Strip comments
			data = data[0:strings.Index(data, "#")]
		}
		if len(strings.TrimSpace(data)) < 1 { //skip empty lines
			continue
		}

		entry := FileEntry{
			Name: data,
		}

		fileList.Deletes = append(fileList.Deletes, entry)
	}

	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}
	return
}
