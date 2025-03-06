#!/bin/bash

# Install pre-commit hook
ln -sf ../../.hooks/commit-msg .git/hooks/commit-msg
chmod +x .git/hooks/commit-msg

# Install pre-commit framework
pip install pre-commit
pre-commit install