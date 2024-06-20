#!/usr/bin/env bash

set -euo pipefail

ansible-playbook -i devops/hosts devops/playbook.yml
