// Copyright 2022 pschou (https://github.com/pschou)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	b64 "encoding/base64"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"

	rpm "github.com/pschou/go-rpm"
)

var version string

func main() {
	flag.Usage = func() {
		_, f := path.Split(os.Args[0])
		fmt.Fprintf(os.Stderr, "rpm-add-checksum,  Version: %s (https://github.com/pschou/rpm-add-checksum)\n\n"+
			"Usage: %s [options] input.rpm output.rpm\n\n", version, f)
		flag.PrintDefaults()
	}

	log.SetFlags(0)
	log.SetPrefix("rpm-add-checksum: ")
	test := flag.Bool("t", false, "Test if SHA256 is present in input file")
	force := flag.Bool("f", false, "Force new file to be written, even if checksum fails")
	inplace := flag.Bool("i", false, "Do upgrade of file hashes in place")
	verbose := flag.Bool("v", false, "Turn on verbose")
	//padding := flag.Bool("a", false, "Add reserve space for signing")

	flag.Parse()

	var outFile string
	if *test {
		// Test only
		if flag.NArg() != 1 {
			fmt.Println("One input file argument required")
			os.Exit(1)
		}
	} else if *inplace {
		if flag.NArg() != 1 {
			fmt.Println("One input file argument required")
			os.Exit(1)
		}
		outFile = flag.Arg(0) + ".inplace"
	} else if flag.NArg() != 2 {
		fmt.Println("Input and Output file required")
		os.Exit(1)
	} else {
		outFile = flag.Arg(1)
	}

	fi, err := os.Open(flag.Arg(0))
	if err != nil {
		log.Fatal(err)
	}

	//buf := bufio.NewReaderSize(fi, 1<<20)
	r := rpm.NewReader(fi)

	var lead *rpm.Lead

	if lead, err = r.Lead(); err != nil {
		log.Fatal(err)
	}

	//ln, _ := lead.Name.MarshalJSON()
	//fmt.Println("Found", string(ln))

	var (
		hdr                *rpm.Header
		hdrs               []*rpm.Header
		found_payload_dgst bool
		payload_dgst_algo  []uint32
	)

	for i := 0; i < 2; i++ {
		hdr, err = r.Next()
		if err != nil {
			break
		}
		//ln, _ := hdr.MarshalJSON()
		//fmt.Println("HDR Found", string(ln))
		//fmt.Println("")
		for _, t := range hdr.Tags {
			if t.Tag == rpm.RPMTAG_PAYLOADDIGESTALGO {
				found_payload_dgst = true
				payload_dgst_algo, _ = t.Int32()
				//fmt.Printf("tag: %#v\n", t)
			}
		}
		//fmt.Printf("Writing header: %#v\n", hdr)
		hdrs = append(hdrs, hdr)
	}

	if *test || (*inplace && found_payload_dgst && !*force) {
		if found_payload_dgst {
			algo := ""
			switch payload_dgst_algo[0] {
			case rpm.PGPHASHALGO_MD5:
				algo = "MD5"
			case rpm.PGPHASHALGO_SHA1:
				algo = "SHA1"
			case rpm.PGPHASHALGO_SHA256:
				algo = "SHA256"
			}
			fmt.Println("Found Digest", algo)
			os.Exit(0)
		}
		fmt.Println("No Digest Found")
		os.Exit(1)
	}

	if *verbose {
		fmt.Println("reading package to hash")
	}
	h_sha1 := sha1.New()
	h_sha256 := sha256.New()
	hashes := io.MultiWriter(h_sha1, h_sha256)

	offset, err := fi.Seek(0, io.SeekCurrent)
	if err != nil {
		log.Fatal("unable to determine current offset")
	}

	var pkgSize int64
	if pkgSize, err = io.Copy(hashes, fi); err != nil {
		fmt.Println("err to hash")
		log.Fatal(err)
	}
	if *verbose {
		fmt.Println("Finished hashing package:")
		fmt.Printf("  %0x\n", h_sha1.Sum(nil))
		fmt.Printf("  %0x\n", h_sha256.Sum(nil))
	}

	//if !found_payload_dgst && found_payload_dgst {
	//}
	if *verbose {
		fmt.Println("Payload header:")
	}
	payloadHdr := rpm.NewPayloadHeader()
	for _, tag := range hdrs[1].Tags {
		switch tag.Tag {
		case rpm.RPMTAG_PAYLOADDIGESTALGO:
		case rpm.RPMTAG_PAYLOADDIGEST:
			d, _ := tag.StringData()
			if *verbose {
				fmt.Println("  Removing", d)
			}
		default:
			payloadHdr.Add(tag)
		}
	}
	payloadHdr.AddStringArray(rpm.RPMTAG_PAYLOADDIGEST, hex.EncodeToString(h_sha256.Sum(nil)))
	if *verbose {
		fmt.Println("  adding", hex.EncodeToString(h_sha256.Sum(nil)))
	}
	payloadHdr.AddInt32(rpm.RPMTAG_PAYLOADDIGESTALGO, rpm.PGPHASHALGO_SHA256)
	//fmt.Println("adding payload digest to header:", hex.EncodeToString(h_sha256.Sum(nil)))
	//ln, _ := hdr.MarshalJSON()
	//fmt.Println("HDR Found", string(ln))

	//for _, tag := range payloadHdr.Tags {
	//	fmt.Println("adding", tag)
	//}

	// Create headers buffer
	if *verbose {
		fmt.Println("Computing new header hash:")
	}
	headers := bytes.NewBufferString("")
	header_sha1 := sha1.New()
	header_sha256 := sha256.New()
	header_md5 := md5.New()
	header_writer := io.MultiWriter(headers, header_sha1, header_sha256, header_md5)
	var payloadHdrSize int64
	if payloadHdrSize, err = rpm.WriteHeaders(header_writer, payloadHdr); err != nil {
		log.Fatal(err)
	}

	// Compare with current infile header
	orig_header_md5 := md5.New()
	rpm.WriteHeaders(orig_header_md5, hdrs[1])

	fi.Seek(offset, io.SeekStart)
	payload_hasher := io.MultiWriter(orig_header_md5, header_md5)
	io.Copy(payload_hasher, fi)

	if *verbose {
		fmt.Println("  sha1:", hex.EncodeToString(header_sha1.Sum(nil)))
		fmt.Println("  sha256:", hex.EncodeToString(header_sha256.Sum(nil)))
		fmt.Println("  md5:", b64.URLEncoding.EncodeToString(header_md5.Sum(nil)))
		fmt.Println("  original md5:", b64.URLEncoding.EncodeToString(orig_header_md5.Sum(nil)))
	}
	orig_md5_compare := b64.URLEncoding.EncodeToString(orig_header_md5.Sum(nil))

	if *verbose {
		fmt.Println("Signature headers:")
	}
	// Look to see is SHA256 is already declared
	Tags := hdrs[0].Tags
	var has_header_sha256 bool
	for _, tag := range Tags {
		if tag.Tag == rpm.RPMTAG_SHA256HEADER {
			if *verbose {
				fmt.Println("  found sha256 in header")
			}
			has_header_sha256 = true
		}
	}

	// Go ahead and declare it
	newHdr := rpm.NewSignatureHeader()
	for _, tag := range Tags {
		//if *padding && tag.Tag == rpm.RPMSIGTAG_RESERVEDSPACE {
		//	continue
		//}
		switch tag.Tag {
		case rpm.RPMTAG_SHA1HEADER:
			d, _ := tag.StringData()
			if *verbose {
				fmt.Println("  updated sha1, prev:", string(d))
			}
			newHdr.AddString(rpm.RPMTAG_SHA1HEADER, hex.EncodeToString(header_sha1.Sum(nil)))
			if !has_header_sha256 {
				if *verbose {
					fmt.Println("  added sha256")
				}
				newHdr.AddString(rpm.RPMTAG_SHA256HEADER, hex.EncodeToString(header_sha256.Sum(nil)))
			}
		case rpm.RPMTAG_SHA256HEADER:
			d, _ := tag.StringData()
			if *verbose {
				fmt.Println("  updated sha256, prev:", string(d))
			}
			newHdr.AddString(rpm.RPMTAG_SHA256HEADER, hex.EncodeToString(header_sha256.Sum(nil)))
		default:
			newHdr.Add(tag)
		}
	}

	// Do a size calculation
	var signatureHdrSize int64
	signatureHdrSize, _ = rpm.WriteHeaders(ioutil.Discard, newHdr)

	var upgrade_check bool

	Tags = newHdr.Tags
	// Write out the new headers with size
	newHdr = rpm.NewSignatureHeader()
	for _, tag := range Tags {
		switch tag.Tag {
		case rpm.RPMSIGTAG_PGP, rpm.RPMTAG_RSAHEADER:
			if *verbose {
				fmt.Println("  removing:", tag)
			}
		case rpm.RPMSIGTAG_SIZE:
			d, _ := tag.Int32()
			if *verbose {
				fmt.Println("  Updated size, prev:", d, "now", uint32(payloadHdrSize+pkgSize), "=", signatureHdrSize, payloadHdrSize, pkgSize)
			}
			newHdr.AddInt32(rpm.RPMSIGTAG_SIZE, uint32(payloadHdrSize+pkgSize))
		case rpm.RPMSIGTAG_MD5:
			d, _ := tag.Bytes()
			if *verbose {
				fmt.Println("  Updated md5, prev:", b64.URLEncoding.EncodeToString(d), "now", b64.URLEncoding.EncodeToString(header_md5.Sum(nil)))
			}
			newHdr.AddBin(rpm.RPMSIGTAG_MD5, header_md5.Sum(nil))
			if b64.URLEncoding.EncodeToString(d) == orig_md5_compare {
				upgrade_check = true
			}
		default:
			newHdr.Add(tag)
		}
	}

	if !upgrade_check {
		if !*force {
			fmt.Println("Error verifying internal MD5 checksum--")
			os.Exit(1)
		} else {
			fmt.Println("Warning, file creation forced with checksum mismatch!")
		}
	}

	fo, err := os.Create(outFile)

	if _, err = lead.WriteTo(fo); err != nil {
		log.Fatal(err)
	}
	//fmt.Println("lead header n", n)

	if _, err = rpm.WriteHeaders(fo, newHdr, payloadHdr); err != nil {
		log.Fatal(err)
	}
	//fmt.Println("header n", n)

	// Copy over the raw data
	fi.Seek(offset, io.SeekStart)
	io.Copy(fo, fi)
	//fmt.Println("payload n", n)

	//dat := make([]byte, 10)
	//fi.Read(dat)
	//fmt.Println("offset", offset, "buf", dat)
	//buf.WriteTo(fo)

	/*
			if *jd {
				jw := json.NewEncoder(os.Stdout)
				if err := jw.Encode(h); err != nil {
					log.Fatal(err)
				}
				os.Exit(0)
			}

		if err := dump(os.Stdout, *fl, h...); err != nil {
			log.Fatal(err)
		}
	*/

	if err != nil && !errors.Is(err, io.EOF) {
		log.Fatalf("error: %v", err)
	}
	if *inplace {
		os.Rename(outFile, flag.Arg(0))
	}
}
