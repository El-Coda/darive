package main

import (
	"archive/zip"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type empty struct {
}

type File struct {
	Name  string
	Path  string
	IsDir bool
}

func main() {
	http.HandleFunc("/", index)
	http.HandleFunc("/open", index)
	http.HandleFunc("/upload", upload)
	http.HandleFunc("/download/", download)
	log.Fatal(http.ListenAndServe("localhost:8000", nil))
}

func index(w http.ResponseWriter, r *http.Request) {
	dirPath := "./uploads/"
	path := r.URL.Query().Get("path")
	fmt.Println(path)
	entries, err := os.ReadDir(dirPath + path)
	if err != nil {
		http.Error(w, "unable to read files", http.StatusInternalServerError)
		return
	}

	var files []File
	for _, entry := range entries {
		files = append(files, File{
			Name:  entry.Name(),
			Path:  filepath.Join(path, entry.Name()), // Just the name relative to current dir
			IsDir: entry.IsDir(),
		})
	}

	data := struct {
		Files []File
	}{
		Files: files,
	}

	var indexTempl = template.Must(template.ParseFiles("templates/index.html"))
	err = indexTempl.Execute(w, data)
	if err != nil {
		panic(err)
	}
}

func download(w http.ResponseWriter, r *http.Request) {
	uri := filepath.Join("./uploads", strings.TrimPrefix(r.URL.Path, "/download/"))
	fmt.Println(uri)
	info, err := os.Stat(uri)
	if err != nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	//stream zip folder
	if info.IsDir() {
		folder, err := os.Open(uri)
		if err != nil {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}
		defer folder.Close()

		piper, pipew := io.Pipe()

		go func() {
			zipWriter := zip.NewWriter(pipew)
			err = filepath.Walk(uri, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}

				if info.IsDir() {
					return nil
				}

				relPath, err := filepath.Rel(uri, path)
				if err != nil {
					return err
				}

				fileWriter, err := zipWriter.Create(relPath)
				if err != nil {
					return err
				}

				file, err := os.Open(path)
				if err != nil {
					return err
				}
				defer file.Close()

				_, err = io.Copy(fileWriter, file)
				return err
			})

			zipWriter.Close()
			pipew.CloseWithError(err)
		}()
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", "attachment; filename=\""+folder.Name()+".zip+\"")
		_, err = io.Copy(w, piper)
		if err != nil {
			fmt.Println("failed to stream the zip", err)
		}
	} else {
		fmt.Println(uri)
		file, err := os.Open(uri)
		if err != nil {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}
		defer file.Close()
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", "attachment; filename=\""+file.Name()+"\"")
		http.ServeFile(w, r, uri)
	}
}

func upload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	err := r.ParseMultipartForm(10 << 20)
	if err != nil {
		http.Error(w, "Failed to parse multipart form", http.StatusBadRequest)
	}

	files := r.MultipartForm.File["files"]

	if len(files) == 0 {
		fmt.Fprintf(w, "no files uploaded")
		return
	}

	for _, fileHeader := range files {
		file, err := fileHeader.Open()
		if err != nil {
			fmt.Fprintf(w, "Error opening file %s: %v\n", fileHeader.Filename, err)
			continue
		}
		defer file.Close()

		path := strings.Split(fileHeader.Header.Get("Content-Disposition"), ";")[2]
		path = strings.TrimPrefix(path, " filename=\"")
		path = strings.TrimSuffix(path, "\"")
		path = filepath.Join("./uploads", path)
		fullPath := path
		path = strings.TrimSuffix(path, fileHeader.Filename)
		os.MkdirAll(path, os.ModePerm)
		dst, err := os.Create(fullPath)
		if err != nil {
			fmt.Fprintf(w, "Error creating destination file %s: %v\n", fileHeader.Filename, err)
			continue
		}
		defer dst.Close()

		if _, err := io.Copy(dst, file); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			continue
		}
		fmt.Fprintf(w, "<p> File %s uploaded successfully! </p>", fileHeader.Filename)
	}
}
