package main

import (
	"crypto/rand"
	"flag"
	"fmt"
	"go/build"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"
)

func main() {
	var pkgsList string
	var encKey string
	var outputGopath bool
	var keepTests bool

	flag.StringVar(&pkgsList, "pkgs", "", "comma separated of packages to process symbols. example: github.com,gopkg.in/specific/pkg")
	flag.StringVar(&encKey, "enckey", "", "rename encryption key")
	flag.BoolVar(&outputGopath, "outdir", false, "output a full GOPATH")
	flag.BoolVar(&keepTests, "keeptests", false, "keep _test.go files")

	flag.Parse()

	if len(flag.Args()) != 2 {
		fmt.Fprintln(os.Stderr, "Usage: gobfuscate [flags] pkg_name out_path")
		flag.PrintDefaults()
		os.Exit(1)
	}

	pkgName := flag.Args()[0]
	outPath := flag.Args()[1]

	if encKey == "" {
		buf := make([]byte, 32)
		rand.Read(buf)
		encKey = string(buf)
	}

	pkgFilters := []string{pkgName}
	if pkgsList != "" {
		pkgFilters = append(pkgFilters, strings.Split(pkgsList, ",")...)
	}
	if !obfuscate(keepTests, outputGopath, encKey, pkgName, outPath, pkgFilters) {
		os.Exit(1)
	}
}

func obfuscate(keepTests, outGopath bool, encKey, pkgName, outPath string, pkgFilter []string) bool {
	var newGopath string
	if outGopath {
		newGopath = outPath
		if err := os.Mkdir(newGopath, 0755); err != nil {
			fmt.Fprintln(os.Stderr, "Failed to create destination:", err)
			return false
		}
	} else {
		var err error
		newGopath, err = ioutil.TempDir("", "")
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to create temp dir:", err)
			return false
		}
		defer os.RemoveAll(newGopath)
	}

	log.Println("Copying GOPATH...")

	if !CopyGopath(pkgName, newGopath, keepTests) {
		return false
	}

	enc := &Encrypter{Key: encKey}
	log.Println("Obfuscating symbols...")
	if err := ObfuscateSymbols(newGopath, enc, pkgFilter); err != nil {
		fmt.Fprintln(os.Stderr, "Failed to obfuscate symbols:", err)
		return false
	}
	log.Println("Obfuscating package names...")
	if err := ObfuscatePackageNames(newGopath, enc); err != nil {
		fmt.Fprintln(os.Stderr, "Failed to obfuscate package names:", err)
		return false
	}
	log.Println("Obfuscating strings...")
	if err := ObfuscateStrings(newGopath); err != nil {
		fmt.Fprintln(os.Stderr, "Failed to obfuscate strings:", err)
		return false
	}

	if !outGopath {
		ctx := build.Default
		newPkg := encryptComponents(pkgName, enc)
		cmd := exec.Command("go", "build", `-ldflags=-s -w -extldflags "-static"`, "-o", outPath, newPkg)
		cmd.Env = []string{"GOROOT=" + ctx.GOROOT, "GOARCH=" + ctx.GOARCH,
			"GOOS=" + ctx.GOOS, "GOPATH=" + newGopath, "PATH=" + os.Getenv("PATH")}
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "Failed to compile:", err)
			return false
		}
	}

	return true
}

func encryptComponents(pkgName string, enc *Encrypter) string {
	comps := strings.Split(pkgName, "/")
	for i, comp := range comps {
		comps[i] = enc.Encrypt(comp)
	}
	return strings.Join(comps, "/")
}
