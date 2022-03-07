package main

import (
	"bytes"
	"errors"
	"fmt"
	"golang.org/x/mod/module"
	"google.golang.org/protobuf/compiler/protogen"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	(protogen.Options{}).Run(generate)
}

func generate(gen *protogen.Plugin) error {
	const (
		moduleName = "github.com/joeycumines/sesame"
	)

	schemaDir, err := os.Getwd()
	if err != nil {
		return err
	}

	projectDir, err := filepath.Abs(filepath.Join(schemaDir, `..`))
	if err != nil {
		return err
	}

	// verify projectDir by reading go.mod and checking for moduleName
	// + sets modulePrefix
	var modulePrefix string
	{
		f, err := os.Open(filepath.Join(projectDir, `go.mod`))
		if err != nil {
			return err
		}

		const (
			modFilePrefix = `module ` + moduleName
			minRead       = len(modFilePrefix) + 1 // + trailing newline
			maxRead       = minRead + 5            //  + `/v123` (at most, optional)
		)

		var buf [maxRead]byte

		n, err := io.ReadAtLeast(f, buf[:], minRead)
		if err != nil {
			return err
		}

		if string(buf[:len(modFilePrefix)]) != modFilePrefix {
			return fmt.Errorf(`unexpected go.mod read: bad prefix: %q`, buf[:])
		}

		// extract/validate modulePrefix
		if i := bytes.IndexByte(buf[len(modFilePrefix):n], '\n'); i < 0 {
			return fmt.Errorf(`unexpected go.mod read: missing newline: %q`, buf[:])
		} else if i > 0 {
			// there should be a version suffix (v2 onwards)
			prefix, pathMajor, ok := module.SplitPathVersion(string(buf[len(modFilePrefix)-len(moduleName) : len(modFilePrefix)+i]))
			if !ok {
				return fmt.Errorf(`unexpected go.mod read: bad module version: %q`, buf[:])
			}
			if prefix != moduleName {
				return fmt.Errorf(`unexpected go.mod read: bad module prefix: %q`, buf[:])
			}
			if pathMajor == `` {
				return fmt.Errorf(`unexpected go.mod read: unexpected module version: %q`, buf[:])
			}
			modulePrefix = prefix + pathMajor + `/`
		} else {
			modulePrefix = moduleName + `/`
		}
	}

	for _, file := range gen.Files {
		if !file.Generate {
			continue
		}

		if !strings.HasPrefix(string(file.GoImportPath), modulePrefix) {
			return fmt.Errorf(`file with import path %s missing module prefix %q: %s`, file.GoImportPath, modulePrefix, file.Desc.Path())
		}

		fileDir := filepath.Join(projectDir, string(file.GoImportPath[len(modulePrefix):]))

		if err := os.MkdirAll(fileDir, 0755); err != nil {
			return err
		}

		for _, fileSuffix := range [...]string{
			`.pb.go`,
			`_grpc.pb.go`,
			`_copy.pb.go`,
		} {
			filePath := file.GeneratedFilenamePrefix + fileSuffix

			stat, err := os.Stat(filePath)
			switch {
			case errors.Is(err, fs.ErrNotExist):
				continue
			case err != nil:
				return err
			case stat.IsDir():
				return fmt.Errorf(`file is dir: %s`, filePath)
			}

			err = os.Rename(filePath, filepath.Join(fileDir, filepath.Base(filePath)))
			switch {
			case errors.Is(err, fs.ErrNotExist):
				continue
			case err != nil:
				return err
			}
		}
	}

	return nil
}
