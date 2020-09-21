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

package cmd

import (
	"context"
	"log"
	"strings"

	"github.com/apigee/registry/cmd/registry/core"
	"github.com/apigee/registry/connection"
	"github.com/apigee/registry/rpc"
	"github.com/apigee/registry/server/names"
	"github.com/golang/protobuf/ptypes/any"
	metrics "github.com/googleapis/gnostic/metrics"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/proto"
)

var vocabularyFilter string

func init() {
	rootCmd.AddCommand(vocabularyCmd)
	vocabularyCmd.PersistentFlags().StringVar(&vocabularyFilter, "filter", "", "filter vocabulary arguments")
}

// vocabularyCmd represents the vocabulary command
var vocabularyCmd = &cobra.Command{
	Use:   "vocabulary",
	Short: "Analyze API vocabularies.",
	Long:  `Analyze API vocabularies.`,
}

func collectInputs(ctx context.Context, client connection.Client, args []string, filter string) []*metrics.Vocabulary {
	name := args[0]
	inputs := make([]*metrics.Vocabulary, 0)
	if m := names.PropertyRegexp().FindStringSubmatch(name); m != nil {
		// Iterate through a collection of properties and summarize each.
		err := core.ListProperties(ctx, client, m, filter, func(property *rpc.Property) {
			switch v := property.GetValue().(type) {
			case *rpc.Property_MessageValue:
				if v.MessageValue.TypeUrl == "gnostic.metrics.Vocabulary" {
					vocab := &metrics.Vocabulary{}
					err := proto.Unmarshal(v.MessageValue.Value, vocab)
					if err != nil {
						log.Printf("%+v", err)
					} else {
						inputs = append(inputs, vocab)
					}
				} else {
					log.Printf("not a vocabulary %s\n", property.Name)
				}
			default:
				log.Printf("not a vocabulary %s\n", property.Name)
			}
		})
		if err != nil {
			log.Fatalf("%s", err.Error())
		}
	}
	return inputs
}

func setVocabularyToProperty(ctx context.Context, client connection.Client, output *metrics.Vocabulary, outputPropertyName string) {
	parts := strings.Split(outputPropertyName, "/properties/")
	subject := parts[0]
	relation := parts[1]
	messageData, err := proto.Marshal(output)
	property := &rpc.Property{
		Subject:  subject,
		Relation: relation,
		Name:     subject + "/properties/" + relation,
		Value: &rpc.Property_MessageValue{
			MessageValue: &any.Any{
				TypeUrl: "gnostic.metrics.Vocabulary",
				Value:   messageData,
			},
		},
	}
	err = core.SetProperty(ctx, client, property)
	if err != nil {
		log.Fatalf("%s", err.Error())
	}
}
