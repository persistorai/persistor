#!/usr/bin/env bash
# install.sh — Install Persistor on a fresh system
#
# Usage:
#   ./install.sh              # Interactive install (prompts for config)
#   ./install.sh --defaults   # Non-interactive with sane defaults
#   ./install.sh --uninstall  # Remove installed files (keeps data)
#
# Prerequisites:
#   - Go 1.25+
#   - PostgreSQL 18 with pgvector extension
#   - Ollama running locally
#
# What it does:
#   1. Builds the binary
#   2. Creates system user (persistor)
#   3. Installs binary to /usr/local/bin
#   4. Generates /etc/persistor.env (if not present)
#   5. Installs systemd service
#   6. Enables and starts the service

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BINARY_NAME="persistor-server"
INSTALL_DIR="/usr/local/bin"
SERVICE_FILE="/etc/systemd/system/persistor.service"
ENV_FILE="/etc/persistor.env"
WORK_DIR="/opt/persistor"
SERVICE_USER="persistor"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info()  { echo -e "${GREEN}[INFO]${NC} $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
error() { echo -e "${RED}[ERROR]${NC} $*" >&2; }
die()   { error "$@"; exit 1; }

# ---------- Uninstall ----------

uninstall() {
    info "Uninstalling Persistor..."
    sudo systemctl stop persistor.service 2>/dev/null || true
    sudo systemctl disable persistor.service 2>/dev/null || true
    sudo rm -f "$SERVICE_FILE"
    sudo rm -f "${INSTALL_DIR}/${BINARY_NAME}"
    sudo systemctl daemon-reload
    info "Removed binary, service file, and disabled service."
    info "Kept: ${ENV_FILE}, ${WORK_DIR}, system user '${SERVICE_USER}', and database."
    info "To fully remove: sudo rm -f ${ENV_FILE}; sudo rm -rf ${WORK_DIR}; sudo userdel ${SERVICE_USER}"
}

if [[ "${1:-}" == "--uninstall" ]]; then
    uninstall
    exit 0
fi

INTERACTIVE=true
if [[ "${1:-}" == "--defaults" ]]; then
    INTERACTIVE=false
fi

# ---------- Detect OS ----------

detect_os() {
    if [[ -f /etc/os-release ]]; then
        . /etc/os-release
        OS_ID="${ID:-unknown}"
        OS_LIKE="${ID_LIKE:-$OS_ID}"
    elif [[ "$(uname)" == "Darwin" ]]; then
        OS_ID="macos"
        OS_LIKE="macos"
    else
        OS_ID="unknown"
        OS_LIKE="unknown"
    fi
}

detect_os

# ---------- Preflight ----------

MISSING=()
WARNINGS=()

command -v sudo >/dev/null 2>&1 || die "sudo is required."
command -v systemctl >/dev/null 2>&1 || { [[ "$OS_ID" != "macos" ]] && die "systemd is required."; }

# Check Go
if command -v go >/dev/null 2>&1; then
    GO_VERSION=$(go version | grep -oP 'go\K[0-9]+\.[0-9]+' 2>/dev/null || echo "unknown")
    info "Found Go ${GO_VERSION}"
else
    MISSING+=("go")
fi

# Check PostgreSQL
if command -v psql >/dev/null 2>&1; then
    info "Found PostgreSQL client"
    # Check for pgvector
    if psql -h localhost -U postgres -c "SELECT 1 FROM pg_available_extensions WHERE name='vector'" 2>/dev/null | grep -q 1; then
        info "Found pgvector extension"
    else
        WARNINGS+=("pgvector extension not detected — install it: https://github.com/pgvector/pgvector")
    fi
else
    MISSING+=("postgresql")
fi

# Check Ollama
if curl -sf http://localhost:11434/api/tags >/dev/null 2>&1; then
    info "Ollama is running"
    # Check for embedding model
    if curl -sf http://localhost:11434/api/tags | grep -q "qwen3-embedding"; then
        info "Found qwen3-embedding model"
    else
        WARNINGS+=("qwen3-embedding model not found — run: ollama pull qwen3-embedding:0.6b")
    fi
elif command -v ollama >/dev/null 2>&1; then
    WARNINGS+=("Ollama is installed but not running — start it: ollama serve")
else
    MISSING+=("ollama")
fi

# ---------- Report & offer to install ----------

if [[ ${#WARNINGS[@]} -gt 0 ]]; then
    echo ""
    for w in "${WARNINGS[@]}"; do
        warn "$w"
    done
fi

if [[ ${#MISSING[@]} -gt 0 ]]; then
    echo ""
    error "Missing prerequisites: ${MISSING[*]}"
    echo ""
    echo "Persistor needs these to run. Install them, or let this script try."
    echo ""

    for dep in "${MISSING[@]}"; do
        case "$dep" in
            go)
                echo "  Go 1.25+:"
                if [[ "$OS_ID" == "macos" ]]; then
                    echo "    brew install go"
                elif [[ "$OS_LIKE" == *debian* ]] || [[ "$OS_LIKE" == *ubuntu* ]]; then
                    echo "    sudo apt install -y golang-go   # or download from https://go.dev/dl/"
                elif [[ "$OS_LIKE" == *fedora* ]] || [[ "$OS_LIKE" == *rhel* ]]; then
                    echo "    sudo dnf install -y golang"
                else
                    echo "    https://go.dev/dl/"
                fi
                ;;
            postgresql)
                echo "  PostgreSQL 16+ with pgvector:"
                if [[ "$OS_ID" == "macos" ]]; then
                    echo "    brew install postgresql@16 pgvector"
                elif [[ "$OS_LIKE" == *debian* ]] || [[ "$OS_LIKE" == *ubuntu* ]]; then
                    echo "    sudo apt install -y postgresql postgresql-16-pgvector"
                elif [[ "$OS_LIKE" == *fedora* ]] || [[ "$OS_LIKE" == *rhel* ]]; then
                    echo "    sudo dnf install -y postgresql-server pgvector"
                else
                    echo "    https://www.postgresql.org/download/"
                fi
                ;;
            ollama)
                echo "  Ollama (local embeddings):"
                echo "    curl -fsSL https://ollama.com/install.sh | sh"
                echo "    ollama pull qwen3-embedding:0.6b"
                ;;
        esac
        echo ""
    done

    if $INTERACTIVE; then
        read -rp "Try to install missing prerequisites automatically? [y/N]: " auto_install
        if [[ "$auto_install" =~ ^[Yy] ]]; then
            for dep in "${MISSING[@]}"; do
                case "$dep" in
                    go)
                        info "Installing Go..."
                        if [[ "$OS_ID" == "macos" ]]; then
                            brew install go
                        elif [[ "$OS_LIKE" == *debian* ]] || [[ "$OS_LIKE" == *ubuntu* ]]; then
                            sudo apt update && sudo apt install -y golang-go
                        elif [[ "$OS_LIKE" == *fedora* ]] || [[ "$OS_LIKE" == *rhel* ]]; then
                            sudo dnf install -y golang
                        else
                            die "Can't auto-install Go on this OS. Install manually: https://go.dev/dl/"
                        fi
                        ;;
                    postgresql)
                        info "Installing PostgreSQL + pgvector..."
                        if [[ "$OS_ID" == "macos" ]]; then
                            brew install postgresql@16 pgvector
                            brew services start postgresql@16
                        elif [[ "$OS_LIKE" == *debian* ]] || [[ "$OS_LIKE" == *ubuntu* ]]; then
                            sudo apt update && sudo apt install -y postgresql postgresql-16-pgvector
                            sudo systemctl enable --now postgresql
                        elif [[ "$OS_LIKE" == *fedora* ]] || [[ "$OS_LIKE" == *rhel* ]]; then
                            sudo dnf install -y postgresql-server pgvector
                            sudo postgresql-setup --initdb
                            sudo systemctl enable --now postgresql
                        else
                            die "Can't auto-install PostgreSQL on this OS. Install manually."
                        fi
                        ;;
                    ollama)
                        info "Installing Ollama..."
                        curl -fsSL https://ollama.com/install.sh | sh
                        info "Pulling embedding model..."
                        ollama pull qwen3-embedding:0.6b
                        ;;
                esac
            done
            info "Prerequisites installed. Continuing..."
        else
            die "Install the prerequisites above and re-run this script."
        fi
    else
        die "Missing prerequisites. Run interactively or install manually."
    fi
fi

# Final verification
command -v go >/dev/null 2>&1 || die "Go still not found after install attempt."

# ---------- Build ----------

info "Building ${BINARY_NAME}..."
cd "$SCRIPT_DIR"
make build

if [[ ! -f "bin/${BINARY_NAME}" ]]; then
    die "Build failed — bin/${BINARY_NAME} not found"
fi

info "Build successful ($(cat VERSION))"

# ---------- System user ----------

if id "$SERVICE_USER" &>/dev/null; then
    info "User '${SERVICE_USER}' already exists"
else
    info "Creating system user '${SERVICE_USER}'..."
    sudo useradd -r -s /usr/sbin/nologin "$SERVICE_USER"
fi

# ---------- Install binary ----------

info "Installing binary to ${INSTALL_DIR}..."
sudo cp "bin/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
sudo chmod 755 "${INSTALL_DIR}/${BINARY_NAME}"

# ---------- Working directory ----------

if [[ ! -d "$WORK_DIR" ]]; then
    info "Creating working directory ${WORK_DIR}..."
    sudo mkdir -p "$WORK_DIR"
    sudo chown "$SERVICE_USER":"$SERVICE_USER" "$WORK_DIR"
fi

# ---------- Environment file ----------

if [[ -f "$ENV_FILE" ]]; then
    info "Environment file ${ENV_FILE} already exists — skipping"
else
    info "Generating ${ENV_FILE}..."

    DB_URL="postgres://persistor:CHANGE_ME@localhost:5432/persistor?sslmode=disable"
    ENC_KEY=""
    PORT="3030"

    if $INTERACTIVE; then
        echo ""
        read -rp "Database URL [${DB_URL}]: " input
        DB_URL="${input:-$DB_URL}"

        read -rp "Port [${PORT}]: " input
        PORT="${input:-$PORT}"

        echo ""
        info "Generating random encryption key..."
    fi

    # Generate a random 32-byte (64 hex char) encryption key
    ENC_KEY=$(openssl rand -hex 32)

    sudo tee "$ENV_FILE" > /dev/null <<EOF
# Persistor environment — generated by install.sh
# Permissions: 600, owned by root
DATABASE_URL=${DB_URL}
PORT=${PORT}
LISTEN_HOST=127.0.0.1
ENCRYPTION_PROVIDER=static
ENCRYPTION_KEY=${ENC_KEY}
OLLAMA_URL=http://localhost:11434
EMBEDDING_MODEL=qwen3-embedding:0.6b
LOG_LEVEL=info
CORS_ORIGINS=http://localhost:3002
EOF
    sudo chmod 600 "$ENV_FILE"
    sudo chown root:root "$ENV_FILE"
    info "Environment file written. Edit ${ENV_FILE} to configure database credentials."
    warn "IMPORTANT: Update DATABASE_URL with your actual database password!"
fi

# ---------- Systemd service ----------

info "Installing systemd service..."
sudo cp "${SCRIPT_DIR}/systemd/persistor.service" "$SERVICE_FILE"
sudo systemctl daemon-reload
sudo systemctl enable persistor.service

# ---------- Start ----------

if $INTERACTIVE; then
    echo ""
    read -rp "Start persistor now? [Y/n]: " start_now
    start_now="${start_now:-Y}"
else
    start_now="Y"
fi

if [[ "$start_now" =~ ^[Yy] ]]; then
    info "Starting persistor..."
    sudo systemctl start persistor.service
    sleep 2
    if systemctl is-active --quiet persistor.service; then
        info "Persistor is running!"
        # Quick health check
        if curl -sf http://localhost:${PORT:-3030}/api/v1/health >/dev/null 2>&1; then
            info "Health check passed ✓"
        else
            warn "Service is running but health endpoint not responding yet — check logs: journalctl -u persistor -f"
        fi
    else
        error "Service failed to start. Check logs: journalctl -u persistor -e"
        exit 1
    fi
else
    info "Skipped. Start manually: sudo systemctl start persistor"
fi

echo ""
info "Installation complete!"
echo "  Binary:  ${INSTALL_DIR}/${BINARY_NAME}"
echo "  Config:  ${ENV_FILE}"
echo "  Service: persistor.service"
echo "  Logs:    journalctl -u persistor -f"
echo "  Health:  curl http://localhost:${PORT:-3030}/api/v1/health"
