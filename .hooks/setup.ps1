# Get the absolute path to the commit-msg hook
$hookPath = Resolve-Path -Path "$PSScriptRoot\commit-msg"

# Ensure the .git/hooks directory exists
$gitRoot = git rev-parse --show-toplevel
$gitHooksDir = Join-Path -Path $gitRoot -ChildPath ".git\hooks"

if (-not (Test-Path -Path $gitHooksDir)) {
    New-Item -ItemType Directory -Path $gitHooksDir -Force
}

# Create the symbolic link (Requires Admin Privileges)
New-Item -ItemType SymbolicLink -Path "$gitHooksDir\commit-msg" -Target $hookPath -Force

# Install pre-commit framework
pip install --upgrade pre-commit
pre-commit install
