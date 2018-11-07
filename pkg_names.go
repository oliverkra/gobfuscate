package main

import (
	"fmt"
	"go/build"
	"go/parser"
	"go/token"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/oliverkra/gobfuscate/pkg/rename"
)

func init() {
	rename.Force = true
}

const GoExtension = ".go"

func ObfuscatePackageNames(gopath string, enc *Encrypter) error {
	ctx := build.Default
	ctx.GOPATH = gopath

	level := 1
	srcDir := filepath.Join(gopath, "src")

	doneChan := make(chan struct{})
	defer close(doneChan)

	for {
		resChan := make(chan string)
		go func() {
			scanLevel(srcDir, level, resChan, doneChan)
			close(resChan)
		}()
		var gotAny bool
		for dirPath := range resChan {
			gotAny = true
			encPath := encryptPackageName(dirPath, enc)
			srcPkg, err := filepath.Rel(srcDir, dirPath)
			if err != nil {
				return err
			}

			dstPkg := srcPkg
			srcPkg = filepath.ToSlash(srcPkg)

			if containsCGO(dirPath) {
				if err := rename.Move(&ctx, srcPkg, srcPkg+"copy", ""); err != nil {
					return fmt.Errorf("package move: %s", err)
				}
				srcPkg = srcPkg + "copy"
			} else {
				dstPkg, err = filepath.Rel(srcDir, encPath)
				if err != nil {
					return err
				}
			}
			dstPkg = filepath.ToSlash(dstPkg)

			if err := rename.Move(&ctx, srcPkg, dstPkg, ""); err != nil {
				return fmt.Errorf("package move: %s", err)
			}
		}
		if !gotAny {
			break
		}
		level++
	}

	return nil
}

func scanLevel(dir string, depth int, res chan<- string, done <-chan struct{}) {
	if depth == 0 {
		select {
		case res <- dir:
		case <-done:
			return
		}
		return
	}
	listing, _ := ioutil.ReadDir(dir)
	for _, item := range listing {
		if item.IsDir() {
			scanLevel(filepath.Join(dir, item.Name()), depth-1, res, done)
		}
		select {
		case <-done:
			return
		default:
		}
	}
}

func encryptPackageName(dir string, enc *Encrypter) string {
	subDir, base := filepath.Split(dir)
	return filepath.Join(subDir, enc.Encrypt(base))
}

func isMainPackage(dir string) bool {
	listing, err := ioutil.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, item := range listing {
		if filepath.Ext(item.Name()) == GoExtension {
			path := filepath.Join(dir, item.Name())
			set := token.NewFileSet()
			file, err := parser.ParseFile(set, path, nil, 0)
			if err != nil {
				return false
			}
			contents, err := ioutil.ReadFile(path)
			if err != nil {
				return false
			}
			fields := strings.Fields(string(contents[int(file.Package)-1:]))
			if len(fields) < 2 {
				return false
			}
			return fields[1] == "main"
		}
	}
	return false
}
