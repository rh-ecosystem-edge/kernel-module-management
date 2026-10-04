#!/bin/bash

set -e

modprobe -d "{{if .DirName}}{{.DirName}}{{else}}/opt{{end}}" "{{.ModuleName}}"
