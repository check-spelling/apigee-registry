#!/bin/sh
#
# Configure an environment to run registry clients with a Cloud Run-based server.
#
# The following assumes you have run `gcloud auth login` and that the current
# gcloud project is the one with your Cloud Run instance.
#

### SERVER CONFIGURATION

# This is used in the Makefile to build and publish your server image.
export REGISTRY_PROJECT_IDENTIFIER=$(gcloud config list --format 'value(core.project)')

### CLIENT CONFIGURATION

# Calls to the Cloud Run service are secure.
unset CLI_REGISTRY_INSECURE

# Get the service address from the gcloud tool.
export CLI_REGISTRY_AUDIENCES=$(gcloud run services describe registry-backend --platform managed --format="value(status.address.url)")
export CLI_REGISTRY_ADDRESS=${CLI_REGISTRY_AUDIENCES#https://}:443

# The auth token is generated for the gcloud logged-in user.
export CLI_REGISTRY_CLIENT_EMAIL=$(gcloud config list account --format "value(core.account)")
export CLI_REGISTRY_TOKEN=$(gcloud auth print-identity-token ${CLI_REGISTRY_CLIENT_EMAIL})
