// Copyright 2020 Google LLC. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/apigee/registry/connection"
	rpcpb "github.com/apigee/registry/rpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const directory = "."
const project = "googleapis"

var registryClient connection.Client

func notFound(err error) bool {
	if err == nil {
		return false
	}
	st, ok := status.FromError(err)
	if !ok {
		return false
	}
	return st.Code() == codes.NotFound
}

func alreadyExists(err error) bool {
	if err == nil {
		return false
	}
	st, ok := status.FromError(err)
	if !ok {
		return false
	}
	return st.Code() == codes.AlreadyExists
}

func main() {
	var err error

	ctx := context.Background()
	registryClient, err = connection.NewClient(ctx)
	if err != nil {
		fmt.Printf("%s\n", err.Error())
		os.Exit(-1)
	}
	completions := make(chan int)
	processes := 0

	r := regexp.MustCompile("v.*[1-9]+.*")

	// walk a directory hierarchy, uploading every API spec that matches a set of expected file names.
	err = filepath.Walk(directory,
		func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				return nil // skip files
			}
			b := path.Base(p)
			if !r.MatchString(b) {
				return nil
			}

			// we need to upload this API spec
			processes++

			go func() {
				err := handleSpec(p)
				if err != nil {
					fmt.Printf("%s\n", err.Error())
				}
				completions <- 1
			}()
			return nil
		})
	if err != nil {
		log.Println(err)
	}
	for i := 0; i < processes; i++ {
		<-completions
		fmt.Printf("COMPLETE: %d\n", i+1)
	}
}

func handleSpec(path string) error {
	// Compute the API name from the path to the spec file.
	name := strings.TrimPrefix(path, directory)
	parts := strings.Split(name, "/")
	version := parts[len(parts)-1]
	api := strings.Join(parts[0:len(parts)-1], "-")
	fmt.Printf("api:%+v version:%+v\n", api, version)
	// Upload the spec for the specified api and version
	return uploadSpec(api, version, "protos.zip", path)
}

func uploadSpec(apiName, version, style, path string) error {
	ctx := context.TODO()
	api := strings.Replace(apiName, "/", "-", -1)
	// If the API does not exist, create it.
	{
		request := &rpcpb.GetApiRequest{
			Name: "projects/" + project + "/apis/" + api,
		}
		_, err := registryClient.GetApi(ctx, request)
		if notFound(err) {
			request := &rpcpb.CreateApiRequest{
				Parent: "projects/" + project,
				ApiId:  api,
				Api: &rpcpb.Api{
					DisplayName: apiName,
				},
			}
			response, err := registryClient.CreateApi(ctx, request)
			if err == nil {
				log.Printf("created %s", response.Name)
			} else if alreadyExists(err) {
				log.Printf("already exists %s/apis/%s", request.Parent, request.ApiId)
			} else {
				log.Printf("failed to create %s/apis/%s: %s",
					request.Parent, request.ApiId, err.Error())
			}
		} else if err != nil {
			return err
		}
	}
	// If the API version does not exist, create it.
	{
		request := &rpcpb.GetVersionRequest{
			Name: "projects/" + project + "/apis/" + api + "/versions/" + version,
		}
		_, err := registryClient.GetVersion(ctx, request)
		if notFound(err) {
			request := &rpcpb.CreateVersionRequest{
				Parent:    "projects/" + project + "/apis/" + api,
				VersionId: version,
				Version:   &rpcpb.Version{},
			}
			response, err := registryClient.CreateVersion(ctx, request)
			if err == nil {
				log.Printf("created %s", response.Name)
			} else if alreadyExists(err) {
				log.Printf("already exists %s/versions/%s", request.Parent, request.VersionId)
			} else {
				log.Printf("failed to create %s/versions/%s: %s",
					request.Parent, request.VersionId, err.Error())
			}
		} else if err != nil {
			return err
		}
	}
	// If the API spec does not exist, create it.
	{
		filename := "protos.zip"
		request := &rpcpb.GetSpecRequest{
			Name: "projects/" + project + "/apis/" + api +
				"/versions/" + version +
				"/specs/" + filename,
		}
		_, err := registryClient.GetSpec(ctx, request)
		if notFound(err) {
			// build a zip archive with the contents of the path
			// https://golangcode.com/create-zip-files-in-go/
			buf, err := zipArchiveOfPath(path)
			if err != nil {
				return err
			}
			request := &rpcpb.CreateSpecRequest{
				Parent: "projects/" + project + "/apis/" + api + "/versions/" + version,
				SpecId: filename,
				Spec: &rpcpb.Spec{
					Style:    "proto+zip",
					Filename: "protos.zip",
					Contents: buf.Bytes(),
				},
			}
			response, err := registryClient.CreateSpec(ctx, request)
			if err == nil {
				log.Printf("created %s", response.Name)
			} else if alreadyExists(err) {
				log.Printf("already exists %s/specs/%s", request.Parent, request.SpecId)
			} else {
				details := fmt.Sprintf("contents-length: %d", len(request.Spec.Contents))
				log.Printf("failed to create %s/specs/%s: %s [%s]",
					request.Parent, request.SpecId, err.Error(), details)
			}

		} else if err != nil {
			return err
		}
	}
	return nil
}

func zipArchiveOfPath(path string) (buf bytes.Buffer, err error) {
	zipWriter := zip.NewWriter(&buf)
	defer zipWriter.Close()

	err = filepath.Walk(path,
		func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			fmt.Printf("<-- %s (%t)\n", p, !info.IsDir())
			if info.IsDir() {
				return nil
			}
			if err = addFileToZip(zipWriter, p); err != nil {
				log.Printf("error adding file %s", err.Error())
				return err
			}
			return nil
		})
	return buf, nil
}

func addFileToZip(zipWriter *zip.Writer, filename string) error {
	log.Printf("AddFileToZip(%s)", filename)
	fileToZip, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer fileToZip.Close()

	// Get the file information
	info, err := fileToZip.Stat()
	if err != nil {
		return err
	}

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}

	// Using FileInfoHeader() above only uses the basename of the file. If we want
	// to preserve the folder structure we can overwrite this with the full path.
	header.Name = filename

	// Change to deflate to gain better compression
	// see http://golang.org/pkg/archive/zip/#pkg-constants
	header.Method = zip.Deflate

	writer, err := zipWriter.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(writer, fileToZip)
	return err
}
