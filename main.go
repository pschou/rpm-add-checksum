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
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"

	rpm "github.com/pschou/go-rpm"
)

var version string

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "rpm-add-sha256,  Version: %s (https://github.com/pschou/rpm-add-sha256)\n\nUsage: %s input.rpm output.rpm\n\n", version, os.Args[0])
		flag.PrintDefaults()
	}

	log.SetFlags(0)
	log.SetPrefix("rpm-add-sha256: ")

	flag.Parse()

	if flag.NArg() != 2 {
		fmt.Println("Input and Output file needed")
		os.Exit(1)
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
		hdr  *rpm.Header
		hdrs []*rpm.Header
		//found_payload_dgst bool
		//found_file_dgst    bool
	)

	for i := 0; i < 2; i++ {
		hdr, err = r.Next()
		if err != nil {
			break
		}
		//ln, _ := hdr.MarshalJSON()
		//fmt.Println("HDR Found", string(ln))
		//fmt.Println("")
		/*
			for _, t := range hdr.Tags {
				fmt.Println("tag", t)
				if t.Tag == rpm.RPMTAG_PAYLOADDIGEST {
					found_payload_dgst = true
				}
				if t.Tag == rpm.RPMTAG_FILEDIGESTALGO {
					found_file_dgst = true
				}
			}*/
		//fmt.Printf("Writing header: %#v\n", hdr)
		hdrs = append(hdrs, hdr)
	}

	offset, err := fi.Seek(0, io.SeekCurrent)
	if err != nil {
		log.Fatal("unable to determine current offset")
	}

	fmt.Println("reading package to hash")
	h_sha1 := sha1.New()
	h_sha256 := sha256.New()
	hashes := io.MultiWriter(h_sha1, h_sha256)

	var pkgSize int64
	if pkgSize, err = io.Copy(hashes, fi); err != nil {
		fmt.Println("err to hash")
		log.Fatal(err)
	}
	fmt.Println("Finished hashing package:")
	fmt.Printf("  %0x\n", h_sha1.Sum(nil))
	fmt.Printf("  %0x\n", h_sha256.Sum(nil))

	//if !found_payload_dgst && found_file_dgst {
	//}
	payloadHdr := rpm.NewPayloadHeader()
	for _, tag := range hdrs[1].Tags {
		switch tag.Tag {
		case rpm.RPMTAG_PAYLOADDIGESTALGO, rpm.RPMTAG_PAYLOADDIGEST:
		default:
			payloadHdr.Add(tag)
		}
	}
	payloadHdr.AddStringArray(rpm.RPMTAG_PAYLOADDIGEST, hex.EncodeToString(h_sha256.Sum(nil)))
	payloadHdr.AddInt32(rpm.RPMTAG_PAYLOADDIGESTALGO, rpm.PGPHASHALGO_SHA256)
	//fmt.Println("adding payload digest to header:", hex.EncodeToString(h_sha256.Sum(nil)))
	//ln, _ := hdr.MarshalJSON()
	//fmt.Println("HDR Found", string(ln))

	//for _, tag := range payloadHdr.Tags {
	//	fmt.Println("adding", tag)
	//}

	// Create headers buffer
	fmt.Println("Computing new header hash:")
	headers := bytes.NewBufferString("")
	header_sha1 := sha1.New()
	header_sha256 := sha256.New()
	header_writer := io.MultiWriter(headers, header_sha1, header_sha256)
	var payloadHdrSize int64
	if payloadHdrSize, err = rpm.WriteHeaders(header_writer, payloadHdr); err != nil {
		log.Fatal(err)
	}
	fmt.Println("  sha1:", hex.EncodeToString(header_sha1.Sum(nil)))
	fmt.Println("  sha256:", hex.EncodeToString(header_sha256.Sum(nil)))

	fmt.Println("Signature headers:")
	// Look to see is SHA256 is already declared
	Tags := hdrs[0].Tags
	var has_header_sha256 bool
	for _, tag := range Tags {
		if tag.Tag == rpm.RPMTAG_SHA256HEADER {
			fmt.Println("found sha256 in header")
			has_header_sha256 = true
		}
	}

	// Go ahead and declare it
	newHdr := rpm.NewSignatureHeader()
	for _, tag := range Tags {
		switch tag.Tag {
		case rpm.RPMTAG_SHA1HEADER:
			d, _ := tag.StringData()
			fmt.Println("  updated sha1, prev:", string(d))
			newHdr.AddString(rpm.RPMTAG_SHA1HEADER, hex.EncodeToString(header_sha1.Sum(nil)))
			if !has_header_sha256 {
				fmt.Println("  added sha256")
				newHdr.AddString(rpm.RPMTAG_SHA256HEADER, hex.EncodeToString(header_sha256.Sum(nil)))
			}
		case rpm.RPMTAG_SHA256HEADER:
			d, _ := tag.StringData()
			fmt.Println("  updated sha256, prev:", string(d))
			newHdr.AddString(rpm.RPMTAG_SHA256HEADER, hex.EncodeToString(header_sha256.Sum(nil)))
		default:
			newHdr.Add(tag)
		}
	}

	// Do a size calculation
	var signatureHdrSize int64
	signatureHdrSize, _ = rpm.WriteHeaders(ioutil.Discard, newHdr)

	Tags = newHdr.Tags
	// Write out the new headers with size
	newHdr = rpm.NewSignatureHeader()
	for _, tag := range Tags {
		switch tag.Tag {
		case rpm.RPMSIGTAG_SIZE:
			d, _ := tag.Int32()
			fmt.Println("Updated size, prev:", d, "now", uint32(payloadHdrSize+pkgSize), "=", signatureHdrSize, payloadHdrSize, pkgSize)
			newHdr.AddInt32(rpm.RPMSIGTAG_SIZE, uint32(payloadHdrSize+pkgSize))
		default:
			newHdr.Add(tag)
		}
	}

	fo, err := os.Create(flag.Arg(1))

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
}
