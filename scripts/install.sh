#!/bin/bash
# 不使用 set -e，脚本通过 error() 函数自行处理错误

# ============================================
# ProxyPanel 一键部署脚本
# 支持: install / update / uninstall / status
#        restart / logs / reset-pwd / backup / restore
# ============================================

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# 路径常量
INSTALL_DIR="/opt/proxy-panel"
CONFIG_FILE="${INSTALL_DIR}/config.yaml"
DB_FILE="${INSTALL_DIR}/data/panel.db"
BACKUP_DIR="${INSTALL_DIR}/backups"
SERVICE_NAME="proxy-panel"
XRAY_SERVICE="xray"
SINGBOX_SERVICE="sing-box"
GITHUB_REPO="mssHYC/proxy-panel"
WEBSITE_CONFIG_DIR="/etc/nginx/proxy-panel"
WEBSITE_ROOT="/var/www/proxy-panel-managed-site"
WEBSITE_UNIT_DIR="/etc/systemd/system"
WEBSITE_SERVICE="proxy-panel-web"

# ============================================
# 辅助函数
# ============================================

info()  { echo -e "${GREEN}[INFO]${NC} $1"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }
step()  { echo -e "${BLUE}[STEP]${NC} $1"; }

check_root() {
    [[ $EUID -ne 0 ]] && error "请使用 root 用户运行此脚本"
}

detect_os() {
    if [[ -f /etc/os-release ]]; then
        # shellcheck source=/dev/null
        . /etc/os-release
        OS=$ID
        VERSION=$VERSION_ID
    else
        error "无法检测操作系统"
    fi
    case "$OS" in
        ubuntu|debian)
            PKG_MGR="apt"
            ;;
        centos|rocky|almalinux|fedora)
            PKG_MGR="yum"
            ;;
        *)
            error "不支持的操作系统: $OS"
            ;;
    esac
    info "检测到操作系统: $OS $VERSION"
}

detect_arch() {
    ARCH=$(uname -m)
    case "$ARCH" in
        x86_64)  ARCH="amd64" ;;
        aarch64) ARCH="arm64" ;;
        armv7l)  ARCH="armv7" ;;
        *)       error "不支持的架构: $ARCH" ;;
    esac
    info "CPU 架构: $ARCH"
}

# 读取用户输入：优先从 /dev/tty 读，避免 `curl | bash` 下 stdin 是管道导致
# read 把脚本自身当作答案。无 tty 时 answer 保持空串，由调用方按默认值决定。
_read_answer() {
    local msg="$1"
    local __var="$2"
    if [[ -r /dev/tty ]]; then
        read -r -p "${msg}" "${__var}" </dev/tty
    else
        # 非交互场景：打印提示但立即返回空串（走默认分支）
        printf '%s' "${msg}"
        eval "${__var}=''"
        echo "(非交互环境，采用默认值)"
    fi
}

confirm() {
    local msg="${1:-确认继续?}"
    local answer
    _read_answer "${msg} [y/N]: " answer
    [[ "$answer" =~ ^[Yy]$ ]]
}

confirm_default_yes() {
    local msg="${1:-确认继续?}"
    local answer
    _read_answer "${msg} [Y/n]: " answer
    [[ -z "$answer" || "$answer" =~ ^[Yy]$ ]]
}

# ============================================
# 安装依赖
# ============================================

install_deps() {
    step "安装系统依赖..."
    if [[ "$PKG_MGR" == "apt" ]]; then
        apt update -y >/dev/null 2>&1
        apt install -y curl wget jq sqlite3 unzip tar >/dev/null 2>&1
    else
        yum install -y curl wget jq sqlite unzip tar >/dev/null 2>&1
    fi
    info "依赖安装完成"
}

# ============================================
# 下载核心组件
# ============================================

download_panel() {
    step "下载 ProxyPanel..."
    local latest_url="https://api.github.com/repos/${GITHUB_REPO}/releases/latest"
    local version

    # 尝试获取最新版本号
    version=$(curl -s "$latest_url" | jq -r '.tag_name' 2>/dev/null) || true

    if [[ -z "$version" || "$version" == "null" ]]; then
        warn "无法获取最新版本，请手动将 proxy-panel 二进制放到 ${INSTALL_DIR}/"
        return 0
    fi

    local download_url="https://github.com/${GITHUB_REPO}/releases/download/${version}/proxy-panel-linux-${ARCH}.tar.gz"
    wget -q --show-progress -O /tmp/proxy-panel.tar.gz "$download_url" || error "下载 ProxyPanel 失败"
    tar -xzf /tmp/proxy-panel.tar.gz -C "${INSTALL_DIR}/"
    chmod +x "${INSTALL_DIR}/proxy-panel"
    rm -f /tmp/proxy-panel.tar.gz
    info "ProxyPanel ${version} 安装完成"
}

download_xray() {
    step "下载 Xray..."
    local version
    # 优先用 redirect 方式获取最新版本号（不消耗 API 限额）
    version=$(curl -sI https://github.com/XTLS/Xray-core/releases/latest | grep -i '^location:' | sed 's/.*\/tag\///' | tr -d '\r\n')
    # 回退到 API 方式
    if [[ -z "$version" || ! "$version" =~ ^v[0-9] ]]; then
        version=$(curl -s https://api.github.com/repos/XTLS/Xray-core/releases/latest | grep '"tag_name"' | head -1 | cut -d'"' -f4)
    fi
    [[ -z "$version" || ! "$version" =~ ^v[0-9] ]] && version="v25.12.31"
    info "Xray 版本: $version"

    # Xray 使用 64/arm64-v8a 等命名
    local xray_arch
    case "$ARCH" in
        amd64) xray_arch="64" ;;
        arm64) xray_arch="arm64-v8a" ;;
        armv7) xray_arch="arm32-v7a" ;;
        *)     xray_arch="$ARCH" ;;
    esac
    local url="https://github.com/XTLS/Xray-core/releases/download/${version}/Xray-linux-${xray_arch}.zip"

    mkdir -p /usr/local/bin
    wget -q --show-progress -O /tmp/xray.zip "$url" || error "下载 Xray 失败"
    unzip -o /tmp/xray.zip -d /tmp/xray/ >/dev/null 2>&1
    cp /tmp/xray/xray /usr/local/bin/xray
    chmod +x /usr/local/bin/xray
    rm -rf /tmp/xray /tmp/xray.zip
    info "Xray 安装完成: $(/usr/local/bin/xray version | head -1)"
}

download_singbox() {
    step "下载 Sing-box..."
    local version
    # 优先用 redirect 方式获取最新版本号（不消耗 API 限额）
    version=$(curl -sI https://github.com/SagerNet/sing-box/releases/latest | grep -i '^location:' | sed 's/.*\/tag\/v//' | tr -d '\r\n')
    # 回退到 API 方式
    if [[ -z "$version" || ! "$version" =~ ^[0-9] ]]; then
        version=$(curl -s https://api.github.com/repos/SagerNet/sing-box/releases/latest | grep '"tag_name"' | head -1 | cut -d'"' -f4 | sed 's/^v//')
    fi
    [[ -z "$version" || ! "$version" =~ ^[0-9] ]] && version="1.13.8"
    info "Sing-box 版本: $version"
    local url="https://github.com/SagerNet/sing-box/releases/download/v${version}/sing-box-${version}-linux-${ARCH}.tar.gz"

    wget -q --show-progress -O /tmp/singbox.tar.gz "$url" || {
        warn "下载 Sing-box 失败 (可选组件，不影响使用)"
        return 0
    }

    tar -xzf /tmp/singbox.tar.gz -C /tmp/ >/dev/null 2>&1
    cp /tmp/sing-box-*/sing-box /usr/local/bin/sing-box
    chmod +x /usr/local/bin/sing-box
    rm -rf /tmp/sing-box-* /tmp/singbox.tar.gz
    info "Sing-box ${version} 安装完成"
}

# ============================================
# TLS 证书相关函数
# ============================================

# 安装 acme.sh (方案 1-4 共用)
install_acme() {
    # 通配符域名 (*.example.com) 含 *，不能直接拼进邮箱，去掉开头的 *.
    local acct_email="admin@${DOMAIN#\*.}"
    if [[ ! -f ~/.acme.sh/acme.sh ]]; then
        info "安装 acme.sh..."
        curl -s https://get.acme.sh | sh -s email="$acct_email" 2>/dev/null || {
            warn "acme.sh 安装失败，请检查网络"
            return 1
        }
    fi
    # 设置默认 CA 为 Let's Encrypt
    ~/.acme.sh/acme.sh --set-default-ca --server letsencrypt 2>/dev/null || true
    return 0
}

# 安装证书到指定路径
install_cert() {
    local ecc_flag=""
    [[ "$1" == "ecc" ]] && ecc_flag="--ecc"
    ~/.acme.sh/acme.sh --install-cert -d "$DOMAIN" $ecc_flag \
        --fullchain-file "$CERT_PATH" \
        --key-file "$KEY_PATH" \
        --reloadcmd "systemctl restart ${SERVICE_NAME} 2>/dev/null; systemctl restart ${XRAY_SERVICE} 2>/dev/null" \
        2>/dev/null || warn "证书安装失败"
    chmod 600 "$CERT_PATH" "$KEY_PATH" 2>/dev/null || true
    info "证书已安装到: $CERT_PATH"
    info "续期将自动执行 (acme.sh cron)"
}

# TLS 证书方案交互选择菜单，设置全局变量: TLS_ENABLED, CERT_PATH, KEY_PATH, DOMAIN
setup_tls() {
    echo ""
    echo "选择 TLS 证书方案:"
    echo "  [1] HTTP 验证申请 (standalone，需 80 端口空闲)"
    echo "  [2] Cloudflare DNS API 申请 (支持通配符，兼容 CDN)"
    echo "  [3] DNSPod (腾讯云) DNS API 申请"
    echo "  [4] Aliyun (阿里云) DNS API 申请"
    echo "  [5] 自定义证书 (上传已有证书)"
    echo "  [6] 不使用 TLS (仅本机，通过 SSH 隧道访问)"
    read -p "请选择 [6]: " TLS_MODE
    TLS_MODE=${TLS_MODE:-6}

    TLS_ENABLED="false"
    CERT_PATH=""
    KEY_PATH=""
    DOMAIN=""

    case "$TLS_MODE" in
        1)
            # HTTP standalone 模式
            read -p "域名 (需已解析到本机): " DOMAIN
            [[ -z "$DOMAIN" ]] && error "域名不能为空"
            TLS_ENABLED="true"
            CERT_PATH="${INSTALL_DIR}/certs/${DOMAIN}.crt"
            KEY_PATH="${INSTALL_DIR}/certs/${DOMAIN}.key"
            mkdir -p "${INSTALL_DIR}/certs"

            install_acme || {
                warn "跳过证书申请，安装完成后可手动执行"
                return 0
            }

            info "正在申请证书 (HTTP standalone，需 80 端口空闲)..."
            echo "  域名: $DOMAIN"
            echo ""

            # 自动释放 80 端口
            local pid80
            pid80=$(ss -tlnp | grep ':80 ' | grep -oP 'pid=\K[0-9]+' | head -1)
            if [[ -n "$pid80" ]]; then
                local proc80
                proc80=$(ps -p "$pid80" -o comm= 2>/dev/null || echo "unknown")
                warn "80 端口被 ${proc80}(PID:${pid80}) 占用，临时停止..."
                kill "$pid80" 2>/dev/null || true
                sleep 1
            fi

            ~/.acme.sh/acme.sh --issue -d "$DOMAIN" --standalone --keylength ec-256 --force || {
                warn "证书申请失败！可能原因:"
                warn "  - 域名未正确解析到本机 IP"
                warn "  - 80 端口被占用 (nginx/apache)"
                warn "安装完成后可手动执行:"
                warn "  ~/.acme.sh/acme.sh --issue -d ${DOMAIN} --standalone"
                return 0
            }

            install_cert "ecc"
            info "✅ 证书申请成功"
            ;;

        2)
            # Cloudflare DNS API 模式
            read -p "域名 (支持通配符如 *.example.com): " DOMAIN
            [[ -z "$DOMAIN" ]] && error "域名不能为空"

            # 提取主域名用于证书路径
            MAIN_DOMAIN=$(echo "$DOMAIN" | sed 's/^\*\.//')
            TLS_ENABLED="true"
            CERT_PATH="${INSTALL_DIR}/certs/${MAIN_DOMAIN}.crt"
            KEY_PATH="${INSTALL_DIR}/certs/${MAIN_DOMAIN}.key"
            mkdir -p "${INSTALL_DIR}/certs"

            echo ""
            echo "Cloudflare API Token 获取方式:"
            echo "  Cloudflare Dashboard → My Profile → API Tokens → Create Token"
            echo "  模板选 'Edit zone DNS'，Zone 选你的域名"
            echo ""
            read -sp "Cloudflare API Token (Zone.DNS 权限): " CF_TOKEN
            echo
            [[ -z "$CF_TOKEN" ]] && error "API Token 不能为空"

            # 可选: Zone ID (自动检测或手动输入)
            read -p "Cloudflare Zone ID (留空自动检测): " CF_ZONE_ID

            install_acme || {
                warn "跳过证书申请"
                return 0
            }

            info "正在通过 Cloudflare DNS API 申请证书..."
            export CF_Token="$CF_TOKEN"
            [[ -n "$CF_ZONE_ID" ]] && export CF_Zone_ID="$CF_ZONE_ID"

            if [[ "$DOMAIN" == \** ]]; then
                # 通配符证书: *.example.com + example.com
                ~/.acme.sh/acme.sh --issue -d "$MAIN_DOMAIN" -d "$DOMAIN" --dns dns_cf --keylength ec-256 --force || {
                    warn "证书申请失败，请检查 API Token 权限和域名是否在 Cloudflare"
                    return 0
                }
                DOMAIN="$MAIN_DOMAIN"
            else
                ~/.acme.sh/acme.sh --issue -d "$DOMAIN" --dns dns_cf --keylength ec-256 --force || {
                    warn "证书申请失败，请检查 API Token 权限"
                    return 0
                }
            fi

            install_cert "ecc"
            info "✅ 证书申请成功 (Cloudflare DNS)"
            ;;

        3)
            # DNSPod (腾讯云) DNS API
            read -p "域名: " DOMAIN
            [[ -z "$DOMAIN" ]] && error "域名不能为空"
            TLS_ENABLED="true"
            CERT_PATH="${INSTALL_DIR}/certs/${DOMAIN}.crt"
            KEY_PATH="${INSTALL_DIR}/certs/${DOMAIN}.key"
            mkdir -p "${INSTALL_DIR}/certs"

            echo ""
            echo "DNSPod API Token 获取方式:"
            echo "  https://console.dnspod.cn/account/token/token"
            echo "  创建 API 密钥，获取 ID 和 Token"
            echo ""
            read -sp "DNSPod API ID: " DP_ID
            echo
            read -sp "DNSPod API Token: " DP_KEY
            echo
            [[ -z "$DP_ID" || -z "$DP_KEY" ]] && error "API ID 和 Token 不能为空"

            install_acme || {
                warn "跳过证书申请"
                return 0
            }

            info "正在通过 DNSPod DNS API 申请证书..."
            export DP_Id="$DP_ID"
            export DP_Key="$DP_KEY"

            ~/.acme.sh/acme.sh --issue -d "$DOMAIN" --dns dns_dp --keylength ec-256 --force || {
                warn "证书申请失败，请检查 API Token 和域名是否托管在 DNSPod"
                return 0
            }

            install_cert "ecc"
            info "✅ 证书申请成功 (DNSPod)"
            ;;

        4)
            # Aliyun DNS API
            read -p "域名: " DOMAIN
            [[ -z "$DOMAIN" ]] && error "域名不能为空"
            TLS_ENABLED="true"
            CERT_PATH="${INSTALL_DIR}/certs/${DOMAIN}.crt"
            KEY_PATH="${INSTALL_DIR}/certs/${DOMAIN}.key"
            mkdir -p "${INSTALL_DIR}/certs"

            echo ""
            echo "阿里云 AccessKey 获取方式:"
            echo "  https://ram.console.aliyun.com/manage/ak"
            echo "  建议使用 RAM 子账号，仅授予 DNS 管理权限"
            echo ""
            read -sp "阿里云 AccessKey ID: " ALI_KEY
            echo
            read -sp "阿里云 AccessKey Secret: " ALI_SECRET
            echo
            [[ -z "$ALI_KEY" || -z "$ALI_SECRET" ]] && error "AccessKey 不能为空"

            install_acme || {
                warn "跳过证书申请"
                return 0
            }

            info "正在通过阿里云 DNS API 申请证书..."
            export Ali_Key="$ALI_KEY"
            export Ali_Secret="$ALI_SECRET"

            ~/.acme.sh/acme.sh --issue -d "$DOMAIN" --dns dns_ali --keylength ec-256 --force || {
                warn "证书申请失败，请检查 AccessKey 和域名是否托管在阿里云"
                return 0
            }

            install_cert "ecc"
            info "✅ 证书申请成功 (阿里云 DNS)"
            ;;

        5)
            # 自定义证书
            read -p "域名 (用于标识): " DOMAIN
            [[ -z "$DOMAIN" ]] && error "域名不能为空"
            TLS_ENABLED="true"
            mkdir -p "${INSTALL_DIR}/certs"

            echo ""
            echo "请选择证书来源:"
            echo "  [a] 输入已有证书文件路径"
            echo "  [b] 粘贴证书内容"
            read -p "请选择 [a]: " CERT_SOURCE
            CERT_SOURCE=${CERT_SOURCE:-a}

            if [[ "$CERT_SOURCE" == "b" ]]; then
                CERT_PATH="${INSTALL_DIR}/certs/${DOMAIN}.crt"
                KEY_PATH="${INSTALL_DIR}/certs/${DOMAIN}.key"

                echo "请粘贴证书内容 (PEM 格式，输入 EOF 结束):"
                cert_content=""
                while IFS= read -r line; do
                    [[ "$line" == "EOF" ]] && break
                    cert_content+="${line}"$'\n'
                done
                echo "$cert_content" > "$CERT_PATH"
                chmod 600 "$CERT_PATH"

                echo "请粘贴私钥内容 (PEM 格式，输入 EOF 结束):"
                key_content=""
                while IFS= read -r line; do
                    [[ "$line" == "EOF" ]] && break
                    key_content+="${line}"$'\n'
                done
                echo "$key_content" > "$KEY_PATH"
                chmod 600 "$KEY_PATH"
                info "✅ 证书已保存"
            else
                read -p "证书文件路径 (.crt/.pem): " CERT_PATH
                read -p "私钥文件路径 (.key): " KEY_PATH
                [[ ! -f "$CERT_PATH" ]] && error "证书文件不存在: $CERT_PATH"
                [[ ! -f "$KEY_PATH" ]] && error "私钥文件不存在: $KEY_PATH"
                info "✅ 使用已有证书: $CERT_PATH"
            fi

            warn "注意: 自定义证书需要自行负责续期"
            ;;

        6)
            info "不使用 TLS，面板将仅在本机以 HTTP 模式运行"
            ;;
        *)
            error "无效的选项: $TLS_MODE"
            ;;
    esac

    # TLS 配置失败时终止，不把管理入口降级到明文 HTTP。
    if [[ "$TLS_ENABLED" == "true" && ( ! -f "$CERT_PATH" || ! -f "$KEY_PATH" ) ]]; then
        error "TLS 证书或私钥不存在，请修正路径后重试: $CERT_PATH / $KEY_PATH"
    fi
}

# ============================================
# 证书管理子命令
# ============================================

cert_status() {
    echo "========== 证书状态 =========="

    if [[ ! -f "$CONFIG_FILE" ]]; then
        warn "配置文件不存在: $CONFIG_FILE"
        return
    fi

    local tls_enabled
    tls_enabled=$(grep '^\s*tls:' "$CONFIG_FILE" | head -1 | awk '{print $2}')
    local cert_file
    cert_file=$(grep '^\s*cert:' "$CONFIG_FILE" | head -1 | awk '{print $2}' | tr -d '"')
    local key_file
    key_file=$(grep '^\s*key:' "$CONFIG_FILE" | head -1 | awk '{print $2}' | tr -d '"')

    if [ "$tls_enabled" = "true" ]; then
        echo -e "TLS 状态: ${GREEN}已启用${NC}"
    else
        echo -e "TLS 状态: ${YELLOW}未启用${NC}"
    fi

    if [[ -n "$cert_file" && -f "$cert_file" ]]; then
        echo "证书文件: $cert_file"
        echo "私钥文件: $key_file"

        # 解析证书信息
        local subject
        subject=$(openssl x509 -in "$cert_file" -noout -subject 2>/dev/null | sed 's/subject=//')
        local issuer
        issuer=$(openssl x509 -in "$cert_file" -noout -issuer 2>/dev/null | sed 's/issuer=//')
        local not_after
        not_after=$(openssl x509 -in "$cert_file" -noout -enddate 2>/dev/null | cut -d= -f2)
        local not_before
        not_before=$(openssl x509 -in "$cert_file" -noout -startdate 2>/dev/null | cut -d= -f2)

        echo "域名:     $subject"
        echo "颁发者:   $issuer"
        echo "生效时间: $not_before"
        echo "到期时间: $not_after"

        # 检查是否即将过期 (30 天)
        local expire_epoch
        expire_epoch=$(date -d "$not_after" +%s 2>/dev/null || date -jf "%b %d %H:%M:%S %Y %Z" "$not_after" +%s 2>/dev/null)
        local now_epoch
        now_epoch=$(date +%s)
        if [[ -n "$expire_epoch" ]]; then
            local days_left=$(( (expire_epoch - now_epoch) / 86400 ))
            if [[ $days_left -lt 0 ]]; then
                echo -e "状态:     ${RED}已过期${NC}"
            elif [[ $days_left -lt 30 ]]; then
                echo -e "状态:     ${YELLOW}即将过期 (${days_left} 天后)${NC}"
            else
                echo -e "状态:     ${GREEN}正常 (${days_left} 天后到期)${NC}"
            fi
        fi
    elif [[ -n "$cert_file" ]]; then
        echo -e "证书文件: ${RED}$cert_file (不存在)${NC}"
    else
        echo "证书文件: 未配置"
    fi

    # 检查 acme.sh 自动续期
    if [[ -f ~/.acme.sh/acme.sh ]]; then
        echo ""
        echo "acme.sh:  已安装"
        local cron_exists
        cron_exists=$(crontab -l 2>/dev/null | grep -c acme.sh || true)
        if [ "$cron_exists" -gt 0 ]; then
            echo -e "自动续期: ${GREEN}已配置${NC}"
        else
            echo -e "自动续期: ${YELLOW}未配置${NC}"
        fi
    fi

    echo "=============================="
}

cert_setup() {
    check_root

    # 显示当前状态
    cert_status
    echo ""

    # 执行 TLS 设置
    setup_tls

    # 更新 config.yaml
    if [[ -f "$CONFIG_FILE" ]]; then
        sed -i "s|^  tls:.*|  tls: ${TLS_ENABLED}|" "$CONFIG_FILE"
        sed -i "s|^  cert:.*|  cert: \"${CERT_PATH}\"|" "$CONFIG_FILE"
        sed -i "/^  cert:/{ n; s|^  key:.*|  key: \"${KEY_PATH}\"|; }" "$CONFIG_FILE"
        sed -i "s|^  domain:.*|  domain: \"${DOMAIN}\"|" "$CONFIG_FILE"
        info "config.yaml 已更新"
    fi

    # 重启所有服务使证书生效
    stop_all_services
    start_services
    info "✅ 证书设置完成"
}

cert_renew() {
    check_root

    if [[ ! -f ~/.acme.sh/acme.sh ]]; then
        error "acme.sh 未安装，无法续期。如果使用自定义证书，请手动更换。"
    fi

    info "正在续期所有证书..."
    ~/.acme.sh/acme.sh --renew-all --force || {
        warn "续期失败，请检查域名解析和网络"
        return 1
    }

    info "重启服务..."
    systemctl restart ${SERVICE_NAME} 2>/dev/null || true
    systemctl restart ${XRAY_SERVICE} 2>/dev/null || true

    info "✅ 证书续期完成"
    cert_status
}

do_cert() {
    case "${2:-}" in
        setup)   cert_setup ;;
        status)  cert_status ;;
        renew)   cert_renew ;;
        *)
            echo "证书管理命令:"
            echo "  $0 cert setup   - 设置/更换 TLS 证书"
            echo "  $0 cert status  - 查看当前证书信息"
            echo "  $0 cert renew   - 手动续期证书"
            ;;
    esac
}

# ============================================
# 交互式配置
# ============================================

interactive_config() {
    step "配置 ProxyPanel..."
    echo ""

    # 面板端口
    read -p "面板端口 [8080]: " PANEL_PORT
    PANEL_PORT=${PANEL_PORT:-8080}

    # 验证端口范围
    if ! [[ "$PANEL_PORT" =~ ^[0-9]+$ ]] || [[ "$PANEL_PORT" -lt 1 || "$PANEL_PORT" -gt 65535 ]]; then
        error "无效的端口号: $PANEL_PORT"
    fi

    # 管理员用户名
    read -p "管理员用户名 [admin]: " ADMIN_USER
    ADMIN_USER=${ADMIN_USER:-admin}

    # 管理员密码
    while true; do
        read -sp "管理员密码 (≥8位): " ADMIN_PASS
        echo
        if [[ ${#ADMIN_PASS} -ge 8 ]]; then
            read -sp "确认密码: " ADMIN_PASS_CONFIRM
            echo
            if [[ "$ADMIN_PASS" == "$ADMIN_PASS_CONFIRM" ]]; then
                break
            else
                warn "两次密码不一致，请重新输入"
            fi
        else
            warn "密码长度至少 8 位"
        fi
    done

    # TLS 方案选择
    setup_tls

    # Telegram 通知
    echo ""
    read -p "Telegram Bot Token (留空跳过): " TG_TOKEN
    TG_CHAT_ID=""
    TG_ENABLE="false"
    if [[ -n "$TG_TOKEN" ]]; then
        read -p "Telegram Chat ID: " TG_CHAT_ID
        TG_ENABLE="true"
    fi

    # 流量限额
    read -p "服务器总流量限额 GB [1000]: " TRAFFIC_LIMIT
    TRAFFIC_LIMIT=${TRAFFIC_LIMIT:-1000}

    # 生成 JWT Secret
    JWT_SECRET=$(head -c 32 /dev/urandom | base64 | tr -d '=/+' | head -c 32)

    info "配置收集完成"
}

# ============================================
# 生成配置文件
# ============================================

generate_config() {
    mkdir -p "${INSTALL_DIR}/data" "${INSTALL_DIR}/kernel" "${INSTALL_DIR}/certs" "${BACKUP_DIR}"

    cat > "$CONFIG_FILE" <<CFGEOF
server:
  listen: "127.0.0.1"
  metrics_token: ""
  port: ${PANEL_PORT}
  tls: ${TLS_ENABLED}
  cert: "${CERT_PATH}"
  key: "${KEY_PATH}"
  domain: "${DOMAIN}"

database:
  path: ${INSTALL_DIR}/data/panel.db

auth:
  jwt_secret: "${JWT_SECRET}"
  admin_user: "${ADMIN_USER}"
  admin_pass: "${ADMIN_PASS}"
  token_expiry_hours: 24

traffic:
  collect_interval_sec: 60
  server_limit_gb: ${TRAFFIC_LIMIT}
  warn_percent: 80
  reset_cron: "0 0 1 * *"

notify:
  telegram:
    enable: ${TG_ENABLE}
    bot_token: "${TG_TOKEN}"
    chat_id: "${TG_CHAT_ID}"
  wechat:
    enable: false
    webhook_url: ""

kernel:
  xray_path: /usr/local/bin/xray
  xray_config: ${INSTALL_DIR}/kernel/xray.json
  xray_api_port: 10085
  singbox_path: /usr/local/bin/sing-box
  singbox_config: ${INSTALL_DIR}/kernel/singbox.json
  singbox_api_port: 9090

firewall:
  enable: ${FIREWALL_ENABLE:-false}
  backend: "${FIREWALL_BACKEND:-}"
CFGEOF

    chmod 600 "$CONFIG_FILE"
    info "配置文件已生成: $CONFIG_FILE"
}

generate_default_xray_config() {
    # 生成一个最小的 Xray 默认配置，面板启动后会动态管理
    cat > "${INSTALL_DIR}/kernel/xray.json" <<'XRAYEOF'
{
  "log": {
    "loglevel": "warning"
  },
  "api": {
    "tag": "api",
    "services": ["HandlerService", "StatsService"]
  },
  "stats": {},
  "policy": {
    "system": {
      "statsInboundUplink": true,
      "statsInboundDownlink": true
    }
  },
  "inbounds": [
    {
      "tag": "api",
      "port": 10085,
      "listen": "127.0.0.1",
      "protocol": "dokodemo-door",
      "settings": {
        "address": "127.0.0.1"
      }
    }
  ],
  "outbounds": [
    {
      "protocol": "freedom",
      "tag": "direct"
    },
    {
      "protocol": "blackhole",
      "tag": "blocked"
    }
  ],
  "routing": {
    "rules": [
      {
        "inboundTag": ["api"],
        "outboundTag": "api",
        "type": "field"
      }
    ]
  }
}
XRAYEOF
    info "Xray 默认配置已生成"
}

generate_default_singbox_config() {
    cat > "${INSTALL_DIR}/kernel/singbox.json" <<'SBEOF'
{
  "log": {
    "level": "warn"
  },
  "inbounds": [],
  "outbounds": [
    {
      "type": "direct",
      "tag": "direct"
    },
    {
      "type": "block",
      "tag": "block"
    }
  ]
}
SBEOF
    info "Sing-box 默认配置已生成"
}

# ============================================
# systemd 服务配置
# ============================================

setup_systemd() {
    step "配置 systemd 服务..."

    # ProxyPanel 服务
    # Restart=always 是必须：备份恢复 API 通过 os.Exit(0) 让 systemd 拉起，
    # 用 on-failure 的话成功退出不会重启。
    cat > /etc/systemd/system/${SERVICE_NAME}.service <<SVCEOF
[Unit]
Description=ProxyPanel - Proxy Management Panel
After=network.target
Wants=network-online.target

[Service]
Type=simple
User=root
WorkingDirectory=${INSTALL_DIR}
ExecStart=${INSTALL_DIR}/proxy-panel -config ${CONFIG_FILE}
Restart=always
RestartSec=5
LimitNOFILE=65535
StandardOutput=journal
StandardError=journal

# --- Security hardening ---
NoNewPrivileges=yes
PrivateTmp=yes
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths=${INSTALL_DIR}
ProtectKernelTunables=yes
ProtectKernelModules=yes
ProtectKernelLogs=yes
ProtectControlGroups=yes
RestrictSUIDSGID=yes
RestrictNamespaces=yes
LockPersonality=yes
RestrictRealtime=yes
# 面板绑定 8080 等高端口，无需 NET_BIND_SERVICE；根据需要自行放行
SystemCallArchitectures=native

[Install]
WantedBy=multi-user.target
SVCEOF

    # Xray 服务
    cat > /etc/systemd/system/${XRAY_SERVICE}.service <<SVCEOF
[Unit]
Description=Xray Service
After=network.target
Wants=network-online.target

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/xray run -config ${INSTALL_DIR}/kernel/xray.json
Restart=on-failure
RestartSec=5
LimitNOFILE=65535
StandardOutput=journal
StandardError=journal

# --- Security hardening ---
NoNewPrivileges=yes
PrivateTmp=yes
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths=${INSTALL_DIR}/kernel
ProtectKernelTunables=yes
ProtectKernelModules=yes
ProtectKernelLogs=yes
ProtectControlGroups=yes
RestrictSUIDSGID=yes
RestrictNamespaces=yes
LockPersonality=yes
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE CAP_NET_ADMIN

[Install]
WantedBy=multi-user.target
SVCEOF

    # Sing-box 服务 (仅在二进制存在时注册)
    if [[ -x /usr/local/bin/sing-box ]]; then
        cat > /etc/systemd/system/${SINGBOX_SERVICE}.service <<SVCEOF
[Unit]
Description=Sing-box Service
After=network.target
Wants=network-online.target

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/sing-box run -c ${INSTALL_DIR}/kernel/singbox.json
Restart=on-failure
RestartSec=5
LimitNOFILE=65535
StandardOutput=journal
StandardError=journal

# --- Security hardening ---
NoNewPrivileges=yes
PrivateTmp=yes
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths=${INSTALL_DIR}/kernel
ProtectKernelTunables=yes
ProtectKernelModules=yes
ProtectKernelLogs=yes
ProtectControlGroups=yes
RestrictSUIDSGID=yes
RestrictNamespaces=yes
LockPersonality=yes
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE CAP_NET_ADMIN

[Install]
WantedBy=multi-user.target
SVCEOF
    fi

    systemctl daemon-reload
    systemctl enable ${SERVICE_NAME} >/dev/null 2>&1
    systemctl enable ${XRAY_SERVICE} >/dev/null 2>&1
    [[ -f /etc/systemd/system/${SINGBOX_SERVICE}.service ]] && systemctl enable ${SINGBOX_SERVICE} >/dev/null 2>&1

    info "systemd 服务已注册并设为开机启动"
}

# ============================================
# 防火墙配置
# ============================================

setup_firewall() {
    step "配置防火墙..."

    local detected=""
    if command -v ufw &>/dev/null; then
        detected="ufw"
    elif command -v firewall-cmd &>/dev/null; then
        detected="firewalld"
    fi

    # 未检测到 ufw / firewalld 时，在 Debian/Ubuntu 上自动通过 apt-get 安装 ufw
    # 使用 detect_os 已设好的 $PKG_MGR，避免在精简镜像下 `command -v apt` 失败
    if [[ -z "$detected" ]]; then
        if [[ "$PKG_MGR" == "apt" ]]; then
            info "未检测到防火墙工具，尝试通过 apt-get 自动安装 ufw..."
            apt-get update -y >/dev/null 2>&1
            if DEBIAN_FRONTEND=noninteractive apt-get install -y ufw >/dev/null 2>&1 && command -v ufw &>/dev/null; then
                detected="ufw"
                info "ufw 安装成功"
            else
                warn "ufw 自动安装失败，请手动执行: apt-get install -y ufw；本次跳过节点端口同步"
                FIREWALL_ENABLE="false"
                FIREWALL_BACKEND=""
                return
            fi
        else
            warn "未检测到防火墙工具，请手动管理需要公开的节点端口"
            FIREWALL_ENABLE="false"
            FIREWALL_BACKEND=""
            return
        fi
    fi

    # 管理入口默认只监听本机，不自动开放面板端口。

    # 重装场景：若旧 config.yaml 已显式启用防火墙同步，直接继承，不再追问
    local prev_enable=""
    local prev_backend=""
    if [[ -f "$CONFIG_FILE" ]]; then
        prev_enable=$(awk '/^firewall:/{flag=1;next} /^[a-z]/{flag=0} flag && /enable:/{print $2; exit}' "$CONFIG_FILE" 2>/dev/null | tr -d '"')
        prev_backend=$(awk '/^firewall:/{flag=1;next} /^[a-z]/{flag=0} flag && /backend:/{print $2; exit}' "$CONFIG_FILE" 2>/dev/null | tr -d '"')
    fi

    if [[ "$prev_enable" == "true" ]]; then
        FIREWALL_ENABLE="true"
        FIREWALL_BACKEND="${prev_backend:-$detected}"
        info "继承原 config.yaml 的节点端口自动同步设置 (backend=${FIREWALL_BACKEND})"
    else
        # 首次安装：询问是否启用节点端口自动同步
        if confirm_default_yes "是否启用节点端口自动放行（新增/删除节点时自动同步防火墙）?"; then
            FIREWALL_ENABLE="true"
            FIREWALL_BACKEND="$detected"
            info "已启用节点端口自动同步 (backend=$detected)"
        else
            FIREWALL_ENABLE="false"
            FIREWALL_BACKEND=""
            info "已跳过节点端口自动同步（可日后在 config.yaml 的 firewall 段手动开启）"
        fi
    fi

    # 启用同步时，立即从 DB 读取存量节点端口并放行。
    # 避免 systemd 尚未启动 / boot-time EnsureAll 还没跑完时节点不可达的窗口。
    if [[ "$FIREWALL_ENABLE" == "true" && -f "$DB_FILE" ]] && command -v sqlite3 &>/dev/null; then
        local ports
        ports=$(sqlite3 "$DB_FILE" "SELECT DISTINCT port FROM nodes WHERE enable=1" 2>/dev/null)
        if [[ -n "$ports" ]]; then
            local count=0
            while IFS= read -r port; do
                [[ -z "$port" ]] && continue
                if [[ "$detected" == "ufw" ]]; then
                    ufw allow "${port}/tcp" >/dev/null 2>&1
                    ufw allow "${port}/udp" >/dev/null 2>&1
                else
                    firewall-cmd --permanent --add-port="${port}/tcp" >/dev/null 2>&1
                    firewall-cmd --permanent --add-port="${port}/udp" >/dev/null 2>&1
                fi
                count=$((count+1))
            done <<< "$ports"
            [[ "$detected" == "firewalld" ]] && firewall-cmd --reload >/dev/null 2>&1
            info "已补放行 ${count} 个存量节点端口"
        fi
    fi
}

# ============================================
# 防火墙管理子命令 (install.sh firewall enable|disable|status)
# ============================================

# 解析 config.yaml 中 firewall 段的当前值到全局 CURR_FW_ENABLE / CURR_FW_BACKEND
parse_firewall_conf() {
    CURR_FW_ENABLE=""
    CURR_FW_BACKEND=""
    [[ ! -f "$CONFIG_FILE" ]] && return
    CURR_FW_ENABLE=$(awk '/^firewall:/{flag=1;next} /^[a-z]/{flag=0} flag && /enable:/{print $2; exit}' "$CONFIG_FILE" 2>/dev/null | tr -d '"')
    CURR_FW_BACKEND=$(awk '/^firewall:/{flag=1;next} /^[a-z]/{flag=0} flag && /backend:/{print $2; exit}' "$CONFIG_FILE" 2>/dev/null | tr -d '"')
}

# 原地改写 config.yaml 的 firewall 段
# 用法: rewrite_firewall_section <enable:true|false> <backend:ufw|firewalld|"">
rewrite_firewall_section() {
    local new_enable="$1"
    local new_backend="$2"
    [[ ! -f "$CONFIG_FILE" ]] && error "找不到配置文件: $CONFIG_FILE"

    cp "$CONFIG_FILE" "${CONFIG_FILE}.bak"

    # 若已存在 firewall 段，用 awk 原地替换 enable/backend 两行；否则 append 整段
    if grep -q "^firewall:" "$CONFIG_FILE"; then
        awk -v en="$new_enable" -v be="$new_backend" '
            BEGIN{flag=0}
            /^firewall:/{print; flag=1; next}
            flag && /^[a-z]/{flag=0}
            flag && /enable:/{printf "  enable: %s\n", en; next}
            flag && /backend:/{printf "  backend: \"%s\"\n", be; next}
            {print}
        ' "${CONFIG_FILE}.bak" > "$CONFIG_FILE"
    else
        {
            cat "${CONFIG_FILE}.bak"
            echo ""
            echo "firewall:"
            echo "  enable: $new_enable"
            echo "  backend: \"$new_backend\""
        } > "$CONFIG_FILE"
    fi
}

# 同步清理 settings 表中的 firewall 键（避免 DB 值覆盖 yaml 值）
clear_firewall_settings_db() {
    [[ ! -f "$DB_FILE" ]] && return
    command -v sqlite3 &>/dev/null || return
    sqlite3 "$DB_FILE" "DELETE FROM settings WHERE key IN ('firewall_enable','firewall_backend')" 2>/dev/null || true
}

firewall_status() {
    echo "========== 防火墙状态 =========="
    parse_firewall_conf
    if [[ "$CURR_FW_ENABLE" == "true" ]]; then
        echo -e "面板同步开关: ${GREEN}已启用${NC}"
    else
        echo -e "面板同步开关: ${YELLOW}未启用${NC}"
    fi
    echo "config.yaml backend: ${CURR_FW_BACKEND:-(未设置)}"

    local detected=""
    if command -v ufw &>/dev/null; then detected="ufw"
    elif command -v firewall-cmd &>/dev/null; then detected="firewalld"
    fi
    echo "本机可用后端: ${detected:-(未检测到)}"

    if [[ "$detected" == "ufw" ]]; then
        echo ""
        echo "--- ufw 当前规则 ---"
        ufw status numbered 2>/dev/null || echo "(ufw status 无法执行)"
    elif [[ "$detected" == "firewalld" ]]; then
        echo ""
        echo "--- firewalld 当前放行端口 ---"
        firewall-cmd --list-ports 2>/dev/null || echo "(firewall-cmd 无法执行)"
    fi

    if [[ -f "$DB_FILE" ]] && command -v sqlite3 &>/dev/null; then
        echo ""
        echo "--- DB 中 enable=true 节点端口 ---"
        sqlite3 "$DB_FILE" "SELECT DISTINCT port FROM nodes WHERE enable=1 ORDER BY port" 2>/dev/null | paste -sd ', ' -
    fi
    echo "================================"
}

firewall_enable() {
    check_root
    step "启用节点端口自动同步..."

    local detected=""
    if command -v ufw &>/dev/null; then detected="ufw"
    elif command -v firewall-cmd &>/dev/null; then detected="firewalld"
    fi

    # 未检测到时尝试自动安装 ufw (仅 apt)
    if [[ -z "$detected" ]]; then
        if [[ "${PKG_MGR:-}" == "apt" ]] || command -v apt-get &>/dev/null; then
            info "未检测到防火墙工具，尝试通过 apt-get 自动安装 ufw..."
            apt-get update -y >/dev/null 2>&1
            if DEBIAN_FRONTEND=noninteractive apt-get install -y ufw >/dev/null 2>&1 && command -v ufw &>/dev/null; then
                detected="ufw"
                info "ufw 安装成功"
            else
                error "未检测到 ufw / firewalld，且自动安装失败。请手动安装后重试。"
            fi
        else
            error "未检测到 ufw / firewalld。请手动安装 (yum/dnf install firewalld 或 apt-get install ufw) 后重试。"
        fi
    fi

    parse_firewall_conf

    # 写回 config.yaml
    rewrite_firewall_section "true" "$detected"
    info "已写入 config.yaml: firewall.enable=true, backend=$detected"

    # 避免 settings 表旧值覆盖 yaml 新值
    clear_firewall_settings_db

    # 对齐存量节点端口
    if [[ -f "$DB_FILE" ]] && command -v sqlite3 &>/dev/null; then
        local ports
        ports=$(sqlite3 "$DB_FILE" "SELECT DISTINCT port FROM nodes WHERE enable=1" 2>/dev/null)
        if [[ -n "$ports" ]]; then
            local count=0
            while IFS= read -r port; do
                [[ -z "$port" ]] && continue
                if [[ "$detected" == "ufw" ]]; then
                    ufw allow "${port}/tcp" >/dev/null 2>&1
                    ufw allow "${port}/udp" >/dev/null 2>&1
                else
                    firewall-cmd --permanent --add-port="${port}/tcp" >/dev/null 2>&1
                    firewall-cmd --permanent --add-port="${port}/udp" >/dev/null 2>&1
                fi
                count=$((count+1))
            done <<< "$ports"
            [[ "$detected" == "firewalld" ]] && firewall-cmd --reload >/dev/null 2>&1
            info "已补放行 ${count} 个存量节点端口"
        fi
    fi

    info "重启面板服务使配置生效..."
    systemctl restart ${SERVICE_NAME} 2>/dev/null || warn "重启 ${SERVICE_NAME} 失败，请手动执行"

    echo ""
    firewall_status
}

firewall_disable() {
    check_root
    step "关闭节点端口自动同步..."

    if ! confirm "关闭后面板将不再同步防火墙规则；已存在的端口规则不会被回收。继续？"; then
        info "已取消"
        return
    fi

    rewrite_firewall_section "false" ""
    info "已写入 config.yaml: firewall.enable=false"

    clear_firewall_settings_db

    info "重启面板服务使配置生效..."
    systemctl restart ${SERVICE_NAME} 2>/dev/null || warn "重启 ${SERVICE_NAME} 失败，请手动执行"

    echo ""
    firewall_status
}

do_firewall() {
    case "${2:-}" in
        enable)  firewall_enable ;;
        disable) firewall_disable ;;
        status)  firewall_status ;;
        *)
            echo "防火墙管理命令:"
            echo "  $0 firewall enable   - 启用节点端口自动同步 (写入 config.yaml + 重启面板 + 放行存量端口)"
            echo "  $0 firewall disable  - 关闭自动同步 (不主动回收已有规则)"
            echo "  $0 firewall status   - 查看启停状态 / 后端 / 放行端口"
            ;;
    esac
}

# ============================================
# 停止所有服务
# ============================================

stop_all_services() {
    step "停止所有服务..."
    systemctl stop ${SERVICE_NAME} 2>/dev/null || true
    systemctl stop ${XRAY_SERVICE} 2>/dev/null || true
    systemctl stop ${SINGBOX_SERVICE} 2>/dev/null || true
    info "所有服务已停止"
}

# ============================================
# 启动服务
# ============================================

start_services() {
    step "启动服务..."
    systemctl start ${XRAY_SERVICE} 2>/dev/null || warn "Xray 启动失败 (需要先配置节点后重启)"
    [[ -f /etc/systemd/system/${SINGBOX_SERVICE}.service ]] && \
        systemctl start ${SINGBOX_SERVICE} 2>/dev/null || true
    systemctl start ${SERVICE_NAME} 2>/dev/null || warn "ProxyPanel 启动失败，请检查配置"
    info "服务启动完成"
}

# ============================================
# 安装完成信息
# ============================================

print_summary() {
    local ip
    ip=$(curl -s4 --connect-timeout 5 ip.sb 2>/dev/null || curl -s4 --connect-timeout 5 ifconfig.me 2>/dev/null || echo "YOUR_SERVER_IP")
    local protocol="http"
    [[ "$TLS_ENABLED" == "true" ]] && protocol="https"
    local access_host="${DOMAIN:-${ip}}"

    echo ""
    echo "============================================"
    echo -e "${GREEN} ProxyPanel 安装完成!${NC}"
    echo "============================================"
    echo ""
    echo "  本机访问: ${protocol}://127.0.0.1:${PANEL_PORT}"
    echo "  SSH 隧道: ssh -N -L ${PANEL_PORT}:127.0.0.1:${PANEL_PORT} root@${ip}"
    echo "  新安装默认仅监听本机；公网管理/订阅请配置独立 HTTPS 反代和访问控制。"
    echo "  现有安装请核对 config.yaml 的 server.listen；TLS/域名绑定需用对应域名访问。"
    if [[ -n "$DOMAIN" ]]; then
        echo "  域名绑定: ${DOMAIN} (IP 直接访问已禁止)"
    fi
    echo "  管理员:   ${ADMIN_USER}"
    echo "  密码:     ********"
    echo ""
    echo "  安装目录: ${INSTALL_DIR}"
    echo "  配置文件: ${CONFIG_FILE}"
    echo "  数据目录: ${INSTALL_DIR}/data"
    echo ""
    echo "  常用命令:"
    echo "    proxy-panel status     - 查看状态"
    echo "    proxy-panel restart    - 重启服务"
    echo "    proxy-panel logs       - 查看日志"
    echo "    proxy-panel backup     - 备份数据"
    echo "    proxy-panel reset-pwd  - 重置密码"
    echo ""
    echo "============================================"
}

# ============================================
# 安装 CLI 管理命令
# ============================================

install_cli() {
    step "安装 proxy-panel 管理命令..."
    # 将脚本自身复制到 /usr/local/bin/proxy-panel
    local script_path
    script_path=$(readlink -f "${BASH_SOURCE[0]}" 2>/dev/null || realpath "${BASH_SOURCE[0]}" 2>/dev/null || echo "$0")
    cp "$script_path" /usr/local/bin/proxy-panel
    chmod +x /usr/local/bin/proxy-panel
    info "已安装 proxy-panel 命令，可直接使用: proxy-panel status / restart / logs 等"
}

# ============================================
# install 子命令: 完整安装
# ============================================

do_install() {
    echo ""
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}   ProxyPanel 一键安装脚本${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo ""

    check_root
    detect_os
    detect_arch

    # Existing installations use the same staged upgrade/rollback path.
    if [[ -f "${INSTALL_DIR}/proxy-panel" ]]; then
        warn "检测到已安装 ProxyPanel"
        if confirm "是否升级现有安装? (保留配置、数据、证书)"; then
            do_update
        fi
        return
    fi

    install_deps

    interactive_config
    setup_firewall
    generate_config
    generate_default_xray_config
    generate_default_singbox_config
    download_xray
    download_singbox
    download_panel
    install_cli
    setup_systemd
    start_services
    print_summary
    offer_website_setup
}

# ============================================
# Managed HTTPS website and HTTPUpgrade reverse proxy
# ============================================

website_tool() {
    python3 - "$CONFIG_FILE" "$INSTALL_DIR" "$WEBSITE_CONFIG_DIR" "$WEBSITE_ROOT" "$WEBSITE_UNIT_DIR" "$@" <<'PY_WEBSITE'
"""Embedded by install.sh; prepare is read-only, apply runs while services stop."""
from contextlib import closing
import ipaddress
import json
import os
from pathlib import Path
import re
import shutil
import socket
import sqlite3
import ssl
import subprocess
import sys
import tempfile

import yaml


def run(args):
    result = subprocess.run(args, capture_output=True, text=True, timeout=25)
    if result.returncode:
        raise ValueError(" ".join(args[:2]) + ": " + result.stderr.strip() + result.stdout.strip())
    return result.stdout


def quote(value):
    if any(c in str(value) for c in '\n\r\x00'):
        raise ValueError("配置值不能包含换行或 NUL")
    return '"' + str(value).replace('\\', '\\\\').replace('"', '\\"').replace('$', '\\$') + '"'


def domain(value):
    value = value.lower().rstrip('.')
    if len(value) > 253 or not re.fullmatch(r'[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?', value):
        raise ValueError("网站需要有效的域名: " + value)
    for label in value.split('.'):
        if not label or len(label) > 63 or label.startswith('-') or label.endswith('-'):
            raise ValueError("无效域名: " + value)
    try:
        ipaddress.ip_address(value)
    except ValueError:
        return value
    raise ValueError("网站域名不能使用 IP: " + value)


def verify_cert(cert, key, names, ca):
    # A password callback avoids prompting on encrypted private keys.
    ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER).load_cert_chain(cert, key, password=lambda: '')
    for name in names:
        try:
            run(['openssl', 'verify', '-purpose', 'sslserver', '-CAfile', ca,
                 '-untrusted', cert, '-verify_hostname', name, cert])
        except ValueError as exc:
            raise ValueError(f"证书 {cert} 不能验证域名 {name}（域名覆盖、有效期或证书链）。"
                             "请先配置覆盖网站及 CDN/SNI 域名的有效证书，再执行 website setup；保留原节点。") from exc


def interface_ips():
    interfaces = json.loads(run(['ip', '-j', 'address', 'show']))
    result = []
    for interface in interfaces:
        if 'UP' not in interface.get('flags', []):
            continue
        for address in interface.get('addr_info', []):
            ip = ipaddress.ip_address(address['local'])
            if not (ip.is_loopback or ip.is_link_local or ip.is_multicast or ip.is_unspecified):
                result.append(str(ip))
    return sorted(set(result))


def select_listens(original, available):
    address = ipaddress.ip_address(original)
    if address.is_loopback:
        raise ValueError("该节点已监听回环，可能由其他反代管理；不会接管现有网站")
    if address.is_unspecified:
        addresses = [ip for ip in available if address.version == 6 or ipaddress.ip_address(ip).version == 4]
    else:
        addresses = [original] if original in available else []
    if not addresses:
        raise ValueError("找不到该节点对应的本机非回环地址，无法安全绑定 Nginx 443")
    return [f'[{ip}]:443' if ':' in ip else f'{ip}:443' for ip in addresses]


def nginx_config(plan, root):
    listen = '\n'.join('        listen ' + address + ' ssl;' for address in plan['listens'])
    default_listen = listen.replace(' ssl;', ' ssl default_server;')
    host = quote(plan['host']) if plan['host'] else '$http_host'
    return f'''pid /run/proxy-panel-web.pid;
error_log stderr warn;
worker_processes auto;
events {{ worker_connections 1024; }}
http {{
    access_log off;
    server_tokens off;
    default_type text/html;
    charset utf-8;
    server {{
{default_listen}
        ssl_reject_handshake on;
    }}
    server {{
{listen}
        server_name {' '.join(plan['domains'])};
        ssl_certificate {quote(plan['cert'])};
        ssl_certificate_key {quote(plan['key'])};
        ssl_protocols TLSv1.2 TLSv1.3;
        root {quote(root)};
        index index.html;
        location / {{ try_files $uri $uri/ =404; }}
        location = {quote(plan['path'])} {{
            error_page 418 = @proxy_panel_node;
            if ($http_upgrade != "") {{ return 418; }}
            try_files $uri $uri/ =404;
        }}
        location @proxy_panel_node {{
            proxy_pass https://127.0.0.1:443;
            proxy_http_version 1.1;
            proxy_set_header Host {host};
            proxy_set_header Upgrade $http_upgrade;
            proxy_set_header Connection "upgrade";
            proxy_ssl_server_name on;
            proxy_ssl_name {quote(plan['sni'])};
            proxy_ssl_verify on;
            proxy_ssl_verify_depth 5;
            proxy_ssl_trusted_certificate {quote(plan['ca'])};
            proxy_buffering off;
            proxy_read_timeout 3600s;
            proxy_send_timeout 3600s;
        }}
    }}
}}
'''


def unit_path(value):
    return quote(str(value).replace('%', '%%'))


def website_units(nginx, config, cert, key):
    command = unit_path(nginx) + ' -c ' + unit_path(config)
    return {
        'proxy-panel-web.service': f'''[Unit]
Description=ProxyPanel website and HTTPUpgrade reverse proxy
After=network.target xray.service
[Service]
Type=simple
ExecStartPre={command} -t
ExecStart={command} -g "daemon off;"
ExecReload={command} -t
ExecReload=/bin/kill -HUP $MAINPID
KillSignal=SIGQUIT
TimeoutStopSec=10
Restart=on-failure
[Install]
WantedBy=multi-user.target
''',
        'proxy-panel-web-reload.service': f'''[Unit]
Description=Reload ProxyPanel website after certificate renewal
[Service]
Type=oneshot
ExecStart={command} -t
ExecStart=/bin/systemctl reload proxy-panel-web.service
''',
        'proxy-panel-web-cert.path': f'''[Unit]
Description=Watch ProxyPanel website certificate renewal
[Path]
PathChanged={unit_path(cert)}
PathChanged={unit_path(key)}
Unit=proxy-panel-web-reload.service
[Install]
WantedBy=multi-user.target
'''}


PAGE = '''<!doctype html><html lang="zh-CN"><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1"><title>山间手记</title>
<style>body{margin:0;background:#f7f5ef;color:#28392e;font:18px/1.9 system-ui,sans-serif}
main{max-width:760px;margin:10vh auto;padding:32px}h1{font-weight:400;font-size:42px}
article{border-top:1px solid #d7ddcf;margin-top:40px;padding-top:20px}p{color:#687267}</style>
<main><small>山间手记 · FIELD NOTES</small><h1>慢一点，看见沿途的风景。</h1>
<p>把读过的书、走过的小路，以及值得停留片刻的日常，留在这里。</p>
<article><h2>没有目的地的下午</h2><p>沿着熟悉的街道多走一个路口，风景就有了变化。
树影挪到墙边，街角的花店换了一束花。</p></article>
<article><h2>给书页留一点空白</h2><p>合上书，让一个句子陪着自己过完一天，
再回头看，它会生出新的意思。</p></article></main></html>'''


def prepare(config, install, confdir, root, units, stage, website_domain=''):
    cfg = yaml.safe_load(Path(config).read_text())
    database = str(Path(cfg.get('database', {}).get('path', 'data/panel.db')).resolve())
    kernel_path = str(Path(cfg['kernel']['xray_config']).resolve())
    state_path = confdir / 'managed.json'
    managed = json.loads(state_path.read_text()) if state_path.exists() else None
    if not managed and (confdir.exists() or root.exists()):
        raise ValueError("网站目录已存在但不是本脚本托管，拒绝覆盖")
    with closing(sqlite3.connect(Path(database).as_uri() + '?mode=ro', uri=True)) as db:
        rows = db.execute('SELECT id,host,protocol,transport,settings FROM nodes WHERE enable=1 AND port=443 AND kernel_type="xray"').fetchall()
    if len(rows) != 1:
        raise ValueError("需要且只能有一个已启用的 Xray TCP 443 节点；请先在面板创建 HTTPUpgrade + TLS 节点")
    node_id, node_host, protocol, transport, original = rows[0]
    settings = json.loads(original)
    if transport != 'httpupgrade' or protocol not in ('vless', 'vmess') or settings.get('security') != 'tls':
        raise ValueError("website setup 当前支持 VLESS/VMess + HTTPUpgrade + TLS，其他入口保持原样")
    if managed and managed['node_id'] != node_id:
        raise ValueError("托管网站对应的节点已更换，请先检查现有部署")
    path = settings.get('path', '/')
    if not isinstance(path, str) or not re.fullmatch(r'/[A-Za-z0-9/_~.\-]*', path):
        raise ValueError("HTTPUpgrade 服务端路径应只含 /、字母、数字、_、~、.、-，不含查询参数")
    sni = domain(settings.get('sni') or settings.get('serverName') or node_host)
    host = settings.get('host', '')
    if host and (not isinstance(host, str) or re.search(r'[\s/\\";$]', host)):
        raise ValueError("HTTPUpgrade Host 格式无效")
    names = [sni]
    try:
        ipaddress.ip_address(node_host)
    except ValueError:
        names.append(domain(node_host))
    if website_domain:
        names.append(domain(website_domain))
    cert = str(Path(settings.get('cert_path') or settings.get('certPath') or '').resolve())
    key = str(Path(settings.get('key_path') or settings.get('keyPath') or '').resolve())
    ca = ssl.get_default_verify_paths().cafile
    if not ca:
        raise ValueError("找不到系统 CA 证书包，请安装 ca-certificates")
    names = sorted(set(names))
    verify_cert(cert, key, names, ca)
    # Include the management domain as a website alias only when its certificate matches.
    panel_domain = cfg.get('server', {}).get('domain', '')
    if panel_domain and panel_domain not in names:
        try:
            panel_domain = domain(panel_domain)
            verify_cert(cert, key, [panel_domain], ca)
            names.append(panel_domain)
        except (ValueError, ssl.SSLError):
            pass
    before = settings.get('listen', '0.0.0.0')
    if managed and before == '127.0.0.1':
        before = managed['listen_before']
    listens = select_listens(before, interface_ips())
    next_settings = dict(settings, listen='127.0.0.1', alpn=['http/1.1'])
    current = json.loads(Path(kernel_path).read_text())
    inbounds = [i for i in current.get('inbounds', []) if i.get('port') == 443]
    if len(inbounds) != 1 or inbounds[0].get('tag') != 'node-' + str(node_id):
        raise ValueError("运行配置与面板 443 节点不一致，请先同步内核")
    stream = inbounds[0].get('streamSettings', {})
    live = stream.get('httpupgradeSettings', {})
    certificates = stream.get('tlsSettings', {}).get('certificates', [])
    if stream.get('network') != transport or stream.get('security') != 'tls' or live.get('path', '/') != path or live.get('host', '') != host or not certificates:
        raise ValueError("运行配置与面板节点参数不一致，请先同步内核")
    if str(Path(certificates[0]['certificateFile']).resolve()) != cert or str(Path(certificates[0]['keyFile']).resolve()) != key:
        raise ValueError("运行配置与面板证书路径不一致，请先同步内核")
    inbounds[0]['listen'] = '127.0.0.1'
    stream['tlsSettings']['alpn'] = ['http/1.1']
    plan = dict(node_id=node_id, original_settings=original, settings=json.dumps(next_settings),
                database=database, kernel_path=kernel_path, node_host=node_host, protocol=protocol,
                path=path, host=host, sni=sni, domains=sorted(names), cert=cert, key=key, ca=ca,
                listens=listens, listen_before=before, webroot=str(root))
    nginx = shutil.which('nginx')
    if not nginx:
        raise ValueError("请先安装 Nginx")
    config_file = confdir / 'nginx.conf'
    files = {kernel_path: ('xray.json', json.dumps(current), 0o600),
             str(config_file): ('nginx.conf', nginx_config(plan, root), 0o600),
             str(state_path): ('managed.json', json.dumps({k: v for k, v in plan.items() if k not in ('original_settings', 'settings')}), 0o600)}
    if not (root / 'index.html').exists():
        files[str(root / 'index.html')] = ('index.html', PAGE, 0o644)
    for name, text in website_units(nginx, config_file, cert, key).items():
        files[str(units / name)] = (name, text, 0o644)
    stage.mkdir(parents=True, mode=0o700, exist_ok=True)
    for name, text, mode in files.values():
        (stage / name).write_text(text)
        os.chmod(stage / name, mode)
    plan['outputs'] = {dest: {'source': entry[0], 'mode': entry[2]} for dest, entry in files.items()}
    plan['files'] = [database, database + '-wal', database + '-shm'] + list(files)
    (stage / 'plan.json').write_text(json.dumps(plan))
    os.chmod(stage / 'plan.json', 0o600)
    try:
        run([cfg['kernel']['xray_path'], 'run', '-test', '-config', str(stage / 'xray.json')])
    except ValueError as exc:
        (stage / 'xray-check.log').write_text(str(exc))
        raise ValueError('Xray 候选配置检查失败，诊断文件: ' + str(stage / 'xray-check.log')) from exc
    run([nginx, '-t', '-c', str(stage / 'nginx.conf')])
    print('网站预检通过: ' + ', '.join(plan['domains']) + '；保留端口 443、路径和节点凭据')


def apply(stage):
    plan = json.loads((stage / 'plan.json').read_text())
    with closing(sqlite3.connect(plan['database'])) as db, db:
        changed = db.execute('UPDATE nodes SET settings=?, updated_at=CURRENT_TIMESTAMP WHERE id=? AND settings=? AND host=? AND protocol=? AND port=443 AND transport="httpupgrade" AND kernel_type="xray" AND enable=1',
                             (plan['settings'], plan['node_id'], plan['original_settings'], plan['node_host'], plan['protocol']))
        if changed.rowcount != 1:
            raise ValueError("节点在预检后发生变化，拒绝覆盖")
        # The surrounding process uses umask 077; new public content directories
        # need explicit traversal permissions for Nginx workers.
        public = Path(plan['webroot'])
        created = []
        parent = public
        while not parent.exists():
            created.append(parent)
            parent = parent.parent
        public.mkdir(parents=True, mode=0o755, exist_ok=True)
        for parent in created:
            os.chmod(parent, 0o755)
        for dest, entry in plan['outputs'].items():
            target = Path(dest)
            target.parent.mkdir(parents=True, mode=0o755, exist_ok=True)
            handle, temp = tempfile.mkstemp(prefix='.website-', dir=target.parent)
            try:
                with os.fdopen(handle, 'wb') as stream:
                    stream.write((stage / entry['source']).read_bytes())
                os.chmod(temp, entry['mode'])
                os.replace(temp, target)
            finally:
                if os.path.exists(temp):
                    os.unlink(temp)


def health(state):
    plan = json.loads(state.read_text())
    address = plan['listens'][0].rsplit(':', 1)[0].strip('[]')
    for name in plan['domains']:
        ctx = ssl.create_default_context(cafile=plan['ca'])
        ctx.set_alpn_protocols(['http/1.1'])
        for path, upgrade in [('/', False), (plan['path'], True)]:
            with socket.create_connection((address, 443), timeout=5) as conn:
                with ctx.wrap_socket(conn, server_hostname=name) as tls:
                    headers = f'GET {path} HTTP/1.1\r\nHost: {name}\r\nConnection: ' + ('Upgrade\r\nUpgrade: websocket' if upgrade else 'close') + '\r\n\r\n'
                    tls.sendall(headers.encode())
                    response = tls.makefile('rb').readline(8192)
                    expected = b'101' if upgrade else b'200'
                    if response.split()[1:2] != [expected]:
                        raise ValueError(f'{name} {path} 应返回 {expected.decode()}，实际: {response[:160]!r}')
    print('网站 HTTPS 200、HTTPUpgrade 101 和证书验证均通过')


def main():
    config, install, confdir, root, units, action, *args = sys.argv[1:]
    os.umask(0o077)
    os.chdir(install)
    if action == 'prepare':
        prepare(config, install, Path(confdir), Path(root), Path(units), Path(args[0]), args[1] if len(args) > 1 else '')
    elif action == 'apply':
        apply(Path(args[0]))
    elif action == 'health':
        health(Path(confdir) / 'managed.json')
    else:
        raise ValueError('未知网站操作: ' + action)


if __name__ == '__main__':
    try:
        main()
    except Exception as exc:
        print('网站操作失败: ' + str(exc), file=sys.stderr)
        sys.exit(1)
PY_WEBSITE
}

install_website_deps() (
    set -e
    local new_nginx=false temporary_policy=false
    command -v nginx >/dev/null || new_nginx=true
    # Debian packages must not start a default public HTTP site during setup.
    cleanup_website_policy() {
        if [[ "$temporary_policy" == true ]]; then rm -f /usr/sbin/policy-rc.d; fi
    }
    trap cleanup_website_policy EXIT
    if [[ "$PKG_MGR" == apt ]]; then
        if [[ "$new_nginx" == true && ! -e /usr/sbin/policy-rc.d && ! -L /usr/sbin/policy-rc.d ]]; then
            printf '#!/bin/sh\nexit 101\n' > /usr/sbin/policy-rc.d
            chmod 755 /usr/sbin/policy-rc.d
            temporary_policy=true
        fi
        apt-get update
        DEBIAN_FRONTEND=noninteractive apt-get install -y nginx python3 python3-yaml openssl ca-certificates iproute2
    else
        yum install -y nginx python3 python3-pyyaml openssl ca-certificates iproute
    fi
    # The managed site has its own unit/config, leaving existing Nginx sites alone.
    if [[ "$new_nginx" == true ]]; then systemctl disable --now nginx; fi
    python3 -c 'import yaml, sqlite3, ssl'
    local cmd
    for cmd in nginx openssl ip jq flock; do command -v "$cmd" >/dev/null || error "缺少网站部署依赖: $cmd"; done
)

setup_website_selinux() {
    [[ -r /sys/fs/selinux/enforce && "$(cat /sys/fs/selinux/enforce)" == 1 ]] || return 0
    # Keep SELinux enforcing. Permit the HTTPS reverse proxy and label only its certs.
    if ! command -v semanage >/dev/null || ! command -v setsebool >/dev/null; then
        [[ "$PKG_MGR" == yum ]] || { warn "请安装 SELinux 管理工具后重试"; return 1; }
        yum install -y policycoreutils policycoreutils-python-utils || return 1
    fi
    local file pattern
    while IFS= read -r file; do
        case "$file" in /root/*|/home/*) warn "SELinux 下请将证书安装到 /opt/proxy-panel/certs 或 /etc/pki/tls 后重试"; return 1;; esac
        pattern=$(python3 -c 'import re,sys; print(re.escape(sys.argv[1]))' "$file") || return 1
        semanage fcontext -a -t cert_t "$pattern" 2>/dev/null || semanage fcontext -m -t cert_t "$pattern" || return 1
        restorecon "$file" || return 1
    done < <(jq -r '.cert,.key' "$1")
    setsebool -P httpd_can_network_connect on
}

run_website_setup() (
    set -eEo pipefail
    umask 077
    cd "$INSTALL_DIR"
    exec 9>"${INSTALL_DIR}/.update.lock"
    flock -n 9 || { warn "另一个升级或网站部署正在运行"; exit 1; }
    local stage backup phase=prepare file svc index=0 website_domain="${1:-}" active=() enabled=()
    stage=$(mktemp -d "${INSTALL_DIR}/.website.XXXXXX")
    backup="${BACKUP_DIR}/website-$(date +%Y%m%d_%H%M%S)-$$"
    finish_website_setup() {
        local status=$?
        trap - EXIT INT TERM
        if [[ "$phase" == stopped || "$phase" == installed ]]; then
            warn "网站部署失败，正在恢复节点配置及服务..."
            systemctl stop "$WEBSITE_SERVICE" proxy-panel-web-cert.path 2>/dev/null || true
            if stop_update_services && { [[ "$phase" == stopped ]] || restore_update_files "$backup"; } && systemctl daemon-reload && start_update_services "$stage/state.json" && keep_update_services_inactive "$stage/state.json"; then
                for svc in "$WEBSITE_SERVICE" proxy-panel-web-cert.path; do
                    if jq -e --arg svc "$svc" '.active | index($svc) != null' "$stage/state.json" >/dev/null; then systemctl start "$svc" || status=1; fi
                    if ! jq -e --arg svc "$svc" '.enabled | index($svc) != null' "$stage/state.json" >/dev/null; then systemctl disable "$svc" 2>/dev/null || true; fi
                done
                warn "已恢复部署前配置；备份: $backup"
            else
                warn "自动恢复未完成，请检查备份: $backup"
            fi
        fi
        if [[ "$status" == 0 ]]; then rm -rf "$stage"; else warn "网站诊断目录: $stage"; fi
        exit "$status"
    }
    trap finish_website_setup EXIT
    trap 'exit 130' INT
    trap 'exit 143' TERM
    "${INSTALL_DIR}/proxy-panel" -config "$CONFIG_FILE" -check-update "$stage/panel-check" > "$stage/panel-check.log" 2>&1
    website_tool prepare "$stage/check" "$website_domain"
    setup_website_selinux "$stage/check/plan.json"
    for svc in "$SERVICE_NAME" "$XRAY_SERVICE" "$SINGBOX_SERVICE" "$WEBSITE_SERVICE" proxy-panel-web-cert.path; do
        if systemctl is-active --quiet "$svc"; then active+=("$svc"); fi
    done
    for svc in "$WEBSITE_SERVICE" proxy-panel-web-cert.path; do
        if systemctl is-enabled --quiet "$svc"; then enabled+=("$svc"); fi
    done
    jq --argjson active "$(printf '%s\n' "${active[@]}" | jq -Rsc 'split("\n") | map(select(length>0))')" \
       --argjson enabled "$(printf '%s\n' "${enabled[@]}" | jq -Rsc 'split("\n") | map(select(length>0))')" \
       '{files,active:$active,enabled:$enabled}' "$stage/check/plan.json" > "$stage/state.json"
    step "网站与证书预检通过，备份并切换 443 入口..."
    phase=stopped
    if [[ -f "$WEBSITE_CONFIG_DIR/managed.json" ]]; then systemctl stop "$WEBSITE_SERVICE" proxy-panel-web-cert.path; fi
    stop_update_services
    mkdir -p "$backup"
    while IFS= read -r file; do
        if [[ -f "$file" ]]; then cp -p "$file" "$backup/$index"; fi
        index=$((index+1))
    done < <(jq -r '.files[]' "$stage/state.json")
    cp "$stage/state.json" "$backup/state.json"
    "${INSTALL_DIR}/proxy-panel" -config "$CONFIG_FILE" -check-update "$stage/final-panel-check" > "$stage/final-panel-check.log" 2>&1
    website_tool prepare "$stage/final" "$website_domain"
    jq -e -s 'length == 2 and .[0] == .[1]' "$stage/check/plan.json" "$stage/final/plan.json" >/dev/null || { warn "预检期间节点或网站配置变化，请重新部署"; exit 1; }
    phase=installed
    website_tool apply "$stage/final"
    if command -v restorecon >/dev/null; then restorecon -R "$WEBSITE_CONFIG_DIR" "$WEBSITE_ROOT" "$WEBSITE_UNIT_DIR/proxy-panel-web.service" "$WEBSITE_UNIT_DIR/proxy-panel-web-reload.service" "$WEBSITE_UNIT_DIR/proxy-panel-web-cert.path"; fi
    systemctl daemon-reload
    start_update_services "$stage/state.json"
    systemctl start "$WEBSITE_SERVICE"
    local healthy=false attempt
    for attempt in {1..10}; do
        if website_tool health && "${INSTALL_DIR}/proxy-panel" -config "$CONFIG_FILE" -check-health; then healthy=true; break; fi
        sleep 1
    done
    [[ "$healthy" == true ]] || { warn "网站或 HTTPUpgrade 转发验收失败"; exit 1; }
    keep_update_services_inactive "$stage/state.json"
    systemctl enable "$WEBSITE_SERVICE" proxy-panel-web-cert.path
    systemctl start proxy-panel-web-cert.path
    phase=complete
    info "网站和 HTTPUpgrade 反代部署完成；客户端请刷新订阅以同步 ALPN。备份: $backup"
)

do_website() {
    case "${2:-setup}" in
        setup)
            check_root
            detect_os
            [[ -f "$CONFIG_FILE" ]] || error "请先安装面板并创建 HTTPUpgrade + TLS 的 443 节点"
            local svc
            for svc in "$SERVICE_NAME" "$XRAY_SERVICE"; do systemctl is-active --quiet "$svc" || error "$svc 未运行，请先恢复节点服务"; done
            install_website_deps
            local dependency_status=$?
            [[ "$dependency_status" == 0 ]] || return "$dependency_status"
            run_website_setup "${3:-}"
            ;;
        status)
            systemctl --no-pager status "$WEBSITE_SERVICE" proxy-panel-web-cert.path
            ;;
        *) error "用法: proxy-panel website {setup [额外网站域名]|status}" ;;
    esac
}

offer_website_setup() {
    if confirm "是否安装/配置 Nginx 443 网站与 HTTPUpgrade 反代?（需已有 TLS 节点和有效域名证书）"; then
        # Do not put do_website in an `if`/`||`: that disables errexit inside its transaction.
        do_website website setup
        local website_status=$?
        if [[ "$website_status" != 0 ]]; then warn "面板安装/升级已完成，网站尚未配置成功；修正提示后执行 proxy-panel website setup"; fi
    else
        info "创建 HTTPUpgrade + TLS 的 443 节点后，可执行 proxy-panel website setup 自动部署网站和反代"
    fi
}

# ============================================
# update 子命令: 升级 (保留配置和数据)
# ============================================

# Fetch a published asset and verify GitHub's SHA-256 before extraction/execution.
# Core versions come from the candidate panel's tested compatibility manifest.
fetch_update_asset() {
    local repo="$1" tag="$2" name="$3" output="$4" metadata url digest actual
    metadata=$(curl -fsSL --connect-timeout 10 --max-time 30 "https://api.github.com/repos/${repo}/releases/${tag}") || return 1
    url=$(jq -er --arg name "$name" '.assets[] | select(.name==$name) | .browser_download_url' <<< "$metadata") || return 1
    digest=$(jq -er --arg name "$name" '.assets[] | select(.name==$name) | .digest' <<< "$metadata") || return 1
    [[ "$url" == "https://github.com/${repo}/releases/download/"* && "$digest" =~ ^sha256:[a-f0-9]{64}$ ]] || return 1
    curl -fL --retry 2 --connect-timeout 10 --max-time 300 "$url" -o "$output" || return 1
    actual=$(sha256sum "$output") || return 1
    [[ "${actual%% *}" == "${digest#sha256:}" ]]
}

# Copy then rename on the destination filesystem; a partial download/copy never
# replaces the currently executable binary.
replace_update_binary() {
    local source="$1" destination="$2"
    cp "$source" "${destination}.update-new" && chmod 755 "${destination}.update-new" && mv -f "${destination}.update-new" "$destination"
}

restore_update_files() {
    local backup="$1" file index=0
    jq -e '.files | type == "array"' "$backup/state.json" >/dev/null || return 1
    while IFS= read -r file; do
        if [[ -f "$backup/$index" ]]; then
            cp -p "$backup/$index" "${file}.update-restore" && mv -f "${file}.update-restore" "$file" || return 1
        else
            rm -f "$file" || return 1
        fi
        index=$((index+1))
    done < <(jq -r '.files[]' "$backup/state.json")
}

stop_update_services() {
    local svc
    for svc in "$SERVICE_NAME" "$XRAY_SERVICE" "$SINGBOX_SERVICE"; do
        # Stop loaded units even while failed/activating, cancelling Restart= jobs
        # before copying the database or restoring old files.
        if [[ "$(systemctl show -p LoadState --value "$svc")" == "loaded" ]]; then
            systemctl stop "$svc" || return 1
        fi
    done
}

start_update_services() {
    local state="$1" svc
    jq -e '.active | type == "array"' "$state" >/dev/null || return 1
    # Panel starts last because it generates/reloads the core configurations.
    for svc in "$XRAY_SERVICE" "$SINGBOX_SERVICE" "$SERVICE_NAME"; do
        if jq -e --arg svc "$svc" '.active | index($svc) != null' "$state" >/dev/null; then
            systemctl start "$svc" || return 1
        fi
    done
}

keep_update_services_inactive() {
    local state="$1" svc
    # Panel startup can restart an unused core; retain the prior inactive state.
    for svc in "$XRAY_SERVICE" "$SINGBOX_SERVICE"; do
        if ! jq -e --arg svc "$svc" '.active | index($svc) != null' "$state" >/dev/null && systemctl is-active --quiet "$svc"; then
            systemctl stop "$svc" || return 1
        fi
    done

}

run_safe_update() (
    set -eEo pipefail
    umask 077
    cd "$INSTALL_DIR"
    exec 9>"${INSTALL_DIR}/.update.lock"
    flock -n 9 || { warn "另一个升级正在运行"; exit 1; }
    local stage backup phase="prepare" svc i file version xray_arch xray_path singbox_path
    local xray_candidate="" singbox_candidate="" active=()
    stage=$(mktemp -d "${INSTALL_DIR}/.update.XXXXXX")
    backup="${BACKUP_DIR}/update-$(date +%Y%m%d_%H%M%S)-$$"
    finish_update() {
        local status=$?
        trap - EXIT INT TERM
        if [[ "$phase" == "stopped" || "$phase" == "installed" ]]; then
            warn "升级失败，正在恢复旧文件和服务..."
            if stop_update_services && { [[ "$phase" == "stopped" ]] || restore_update_files "$backup"; } && start_update_services "$stage/state.json"; then
                warn "已恢复升级前文件及运行状态，备份: $backup"
            else
                warn "自动恢复未完成，请使用备份 $backup 检查服务；预检日志: $stage"
            fi
        fi
        if [[ "$status" == 0 ]]; then rm -rf "$stage"; else warn "升级已中止，诊断目录: $stage"; fi
        exit "$status"
    }
    trap finish_update EXIT
    trap 'exit 130' INT
    trap 'exit 143' TERM

    step "下载并验证升级候选（当前服务继续运行）..."
    fetch_update_asset "$GITHUB_REPO" latest "proxy-panel-linux-${ARCH}.tar.gz" "$stage/panel.tar.gz"
    tar -xzf "$stage/panel.tar.gz" -C "$stage" proxy-panel
    chmod 755 "$stage/proxy-panel"
    "$stage/proxy-panel" -compatibility > "$stage/compatibility.json"
    if confirm "是否同时更新 Xray 到本版本已验证的内核?"; then
        version=$(jq -er '.xray' "$stage/compatibility.json")
        [[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]
        case "$ARCH" in amd64) xray_arch=64 ;; arm64) xray_arch=arm64-v8a ;; *) exit 1 ;; esac
        fetch_update_asset XTLS/Xray-core "tags/$version" "Xray-linux-${xray_arch}.zip" "$stage/xray.zip"
        unzip -p "$stage/xray.zip" xray > "$stage/xray"
        chmod 755 "$stage/xray"
        xray_candidate="$stage/xray"
    fi
    if confirm "是否同时更新 Sing-box 到本版本已验证的内核?"; then
        version=$(jq -er '."sing-box"' "$stage/compatibility.json")
        [[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]
        fetch_update_asset SagerNet/sing-box "tags/$version" "sing-box-${version#v}-linux-${ARCH}.tar.gz" "$stage/singbox.tar.gz"
        tar -xOzf "$stage/singbox.tar.gz" "sing-box-${version#v}-linux-${ARCH}/sing-box" > "$stage/sing-box"
        chmod 755 "$stage/sing-box"
        singbox_candidate="$stage/sing-box"
    fi
    "$stage/proxy-panel" -config "$CONFIG_FILE" -check-update "$stage/check" -xray-candidate "$xray_candidate" -singbox-candidate "$singbox_candidate" > "$stage/preflight.log" 2>&1
    xray_path=$(jq -er '.xray_path' "$stage/check/manifest.json")
    singbox_path=$(jq -r '.singbox_path' "$stage/check/manifest.json")
    for svc in "$SERVICE_NAME" "$XRAY_SERVICE" "$SINGBOX_SERVICE"; do
        if systemctl is-active --quiet "$svc"; then active+=("$svc"); fi
    done
    jq --arg panel "${INSTALL_DIR}/proxy-panel" --argjson active "$(printf '%s\n' "${active[@]}" | jq -Rsc 'split("\n") | map(select(length>0))')" '.files += [$panel] | .files |= unique | .active=$active' "$stage/check/manifest.json" > "$stage/state.json"
    step "预检通过，停止服务并创建一致备份..."
    phase="stopped"
    stop_update_services
    mkdir -p "$backup"
    i=0
    while IFS= read -r file; do
        if [[ -f "$file" ]]; then cp -p "$file" "$backup/$i"; fi
        i=$((i+1))
    done < <(jq -r '.files[]' "$stage/state.json")
    cp "$stage/state.json" "$backup/state.json"
    # Recheck after stopping: includes any node edits made during download.
    "$stage/proxy-panel" -config "$CONFIG_FILE" -check-update "$stage/final-check" -xray-candidate "$xray_candidate" -singbox-candidate "$singbox_candidate" > "$stage/final-preflight.log" 2>&1
    # jq is already checked before the update; minimal systems may lack cmp.
    jq -e -s 'if length == 2 then .[0] == .[1] else error("expected two preflight manifests") end' \
        "$stage/check/manifest.json" "$stage/final-check/manifest.json" >/dev/null || {
        local comparison_status=$?
        if [[ "$comparison_status" == 1 ]]; then
            warn "预检期间配置路径发生变化，请重新升级"
        else
            warn "无法读取或比较升级预检清单，请检查诊断目录: $stage"
        fi
        exit 1
    }
    phase="installed"
    replace_update_binary "$stage/proxy-panel" "${INSTALL_DIR}/proxy-panel"
    if [[ -n "$xray_candidate" ]]; then replace_update_binary "$xray_candidate" "$xray_path"; fi
    if [[ -n "$singbox_candidate" ]]; then replace_update_binary "$singbox_candidate" "$singbox_path"; fi
    start_update_services "$stage/state.json"
    for i in {1..15}; do
        sleep 1
        local healthy=true
        for svc in "${active[@]}"; do systemctl is-active --quiet "$svc" || healthy=false; done
        if jq -e --arg svc "$SERVICE_NAME" '.active | index($svc) != null' "$stage/state.json" >/dev/null; then
            "${INSTALL_DIR}/proxy-panel" -config "$CONFIG_FILE" -check-health >> "$stage/health.log" 2>&1 || healthy=false
        fi
        if [[ "$healthy" == true ]]; then
            keep_update_services_inactive "$stage/state.json"
            if [[ "$i" -ge 5 ]]; then break; fi
        fi
    done
    [[ "$healthy" == true ]] || { warn "新版本启动/健康检查失败"; exit 1; }
    phase="complete"
    info "升级完成，监听配置已保留。回退备份: $backup"
)

# Explicit recovery entry for a completed update. Restores the database too.
do_rollback_update() (
    check_root
    local backup="$1"
    [[ "$backup" == "${BACKUP_DIR}/update-"* && -f "$backup/state.json" ]] || error "请选择本机 backups/update-* 备份目录"
    warn "回退会恢复升级时的数据库，之后的节点、账号和流量修改将丢失。"
    confirm "确认恢复 $backup ?" || exit 0
    set -eEo pipefail
    exec 9>"${INSTALL_DIR}/.update.lock"
    flock -n 9 || error "另一个升级正在运行"
    stop_update_services
    restore_update_files "$backup"
    start_update_services "$backup/state.json"
    info "已恢复升级前的程序、数据和服务状态"
)

do_update() {
    check_root
    detect_os
    detect_arch
    [[ -f "$CONFIG_FILE" ]] || error "未检测到已安装的 ProxyPanel，请先执行 install"
    local cmd
    for cmd in curl jq sha256sum flock tar unzip; do command -v "$cmd" >/dev/null || error "缺少升级依赖: $cmd"; done
    run_safe_update
    local update_status=$?
    [[ "$update_status" == 0 ]] || return "$update_status"
    offer_website_setup
}

# ============================================
# uninstall 子命令: 卸载
# ============================================

do_uninstall() {
    check_root

    echo ""
    warn "即将卸载 ProxyPanel"

    if ! confirm "确认卸载?"; then
        info "取消卸载"
        exit 0
    fi

    # 停止并禁用服务
    step "停止服务..."
    systemctl stop ${SERVICE_NAME} 2>/dev/null || true
    systemctl stop ${XRAY_SERVICE} 2>/dev/null || true
    systemctl stop ${SINGBOX_SERVICE} 2>/dev/null || true
    systemctl disable ${SERVICE_NAME} 2>/dev/null || true
    systemctl disable ${XRAY_SERVICE} 2>/dev/null || true
    systemctl disable ${SINGBOX_SERVICE} 2>/dev/null || true
    if [[ -f "$WEBSITE_CONFIG_DIR/managed.json" ]]; then
        systemctl disable --now "$WEBSITE_SERVICE" proxy-panel-web-cert.path 2>/dev/null || true
        rm -f "$WEBSITE_UNIT_DIR/proxy-panel-web.service" "$WEBSITE_UNIT_DIR/proxy-panel-web-reload.service" "$WEBSITE_UNIT_DIR/proxy-panel-web-cert.path"
    fi

    # 删除 systemd 服务文件
    rm -f /etc/systemd/system/${SERVICE_NAME}.service
    rm -f /etc/systemd/system/${XRAY_SERVICE}.service
    rm -f /etc/systemd/system/${SINGBOX_SERVICE}.service
    systemctl daemon-reload

    # 是否保留数据
    if confirm "是否保留配置和数据? (方便日后重装)"; then
        info "保留目录: ${INSTALL_DIR}/data 和 ${CONFIG_FILE}"
        # 只删除二进制和内核
        rm -f "${INSTALL_DIR}/proxy-panel"
        rm -rf "${INSTALL_DIR}/kernel"
    else
        warn "删除所有数据..."
        rm -rf "${INSTALL_DIR}"
    fi

    # 删除内核二进制
    rm -f /usr/local/bin/xray
    rm -f /usr/local/bin/sing-box

    echo ""
    info "ProxyPanel 已卸载"
}

# ============================================
# status 子命令: 查看状态
# ============================================

do_status() {
    echo ""
    echo "========================================="
    echo "  ProxyPanel 服务状态"
    echo "========================================="
    echo ""

    # ProxyPanel
    echo -e "${BLUE}[ProxyPanel]${NC}"
    if systemctl is-active --quiet ${SERVICE_NAME} 2>/dev/null; then
        echo -e "  状态: ${GREEN}运行中${NC}"
    else
        echo -e "  状态: ${RED}未运行${NC}"
    fi
    systemctl show ${SERVICE_NAME} --property=ActiveState,SubState,MainPID 2>/dev/null | \
        sed 's/^/  /' || echo "  (服务未注册)"
    echo ""

    # Xray
    echo -e "${BLUE}[Xray]${NC}"
    if systemctl is-active --quiet ${XRAY_SERVICE} 2>/dev/null; then
        echo -e "  状态: ${GREEN}运行中${NC}"
    else
        echo -e "  状态: ${RED}未运行${NC}"
    fi
    systemctl show ${XRAY_SERVICE} --property=ActiveState,SubState,MainPID 2>/dev/null | \
        sed 's/^/  /' || echo "  (服务未注册)"
    echo ""

    # Sing-box
    echo -e "${BLUE}[Sing-box]${NC}"
    if systemctl is-active --quiet ${SINGBOX_SERVICE} 2>/dev/null; then
        echo -e "  状态: ${GREEN}运行中${NC}"
    else
        echo -e "  状态: ${YELLOW}未运行/未安装${NC}"
    fi
    systemctl show ${SINGBOX_SERVICE} --property=ActiveState,SubState,MainPID 2>/dev/null | \
        sed 's/^/  /' || echo "  (服务未注册)"
    echo ""

    if [[ -f "$WEBSITE_CONFIG_DIR/managed.json" ]]; then
        echo -e "${BLUE}[网站 / Nginx]${NC}"
        systemctl show "$WEBSITE_SERVICE" --property=ActiveState,SubState,MainPID 2>/dev/null | sed 's/^/  /'
        echo ""
    fi

    # 磁盘用量
    echo -e "${BLUE}[磁盘用量]${NC}"
    if [[ -d "$INSTALL_DIR" ]]; then
        echo "  安装目录: $(du -sh "$INSTALL_DIR" 2>/dev/null | cut -f1)"
        [[ -f "$DB_FILE" ]] && echo "  数据库:   $(du -sh "$DB_FILE" 2>/dev/null | cut -f1)"
    else
        echo "  (未安装)"
    fi
    echo ""
}

# ============================================
# restart 子命令: 重启服务
# ============================================

do_restart() {
    check_root
    step "重启所有服务..."

    systemctl restart ${XRAY_SERVICE} 2>/dev/null && \
        info "Xray 已重启" || warn "Xray 重启失败"

    if [[ -f /etc/systemd/system/${SINGBOX_SERVICE}.service ]]; then
        systemctl restart ${SINGBOX_SERVICE} 2>/dev/null && \
            info "Sing-box 已重启" || warn "Sing-box 重启失败"
    fi

    systemctl restart ${SERVICE_NAME} 2>/dev/null && \
        info "ProxyPanel 已重启" || warn "ProxyPanel 重启失败"
    if [[ -f "$WEBSITE_CONFIG_DIR/managed.json" ]]; then
        nginx -t -c "$WEBSITE_CONFIG_DIR/nginx.conf" && systemctl restart "$WEBSITE_SERVICE" || warn "网站重启失败，请执行 proxy-panel website status 查看"
    fi

    echo ""
    info "服务重启完成"
}

# ============================================
# logs 子命令: 查看日志
# ============================================

do_logs() {
    local service="${2:-${SERVICE_NAME}}"
    local lines="${3:-100}"

    echo "查看 ${service} 日志 (最近 ${lines} 行, Ctrl+C 退出)..."
    echo ""
    journalctl -u "$service" -n "$lines" -f --no-pager
}

# ============================================
# reset-pwd 子命令: 重置管理员密码
# ============================================

do_reset_pwd() {
    check_root

    [[ ! -f "$CONFIG_FILE" ]] && error "配置文件不存在: $CONFIG_FILE"

    echo ""
    step "重置管理员密码"

    # 显示当前管理员用户名
    # 优先读 DB（WebUI 改名只写 settings 表，不回写 yaml），读不到再回退 yaml
    local current_user=""
    if [[ -f "$DB_FILE" ]] && command -v sqlite3 &>/dev/null; then
        current_user=$(sqlite3 "$DB_FILE" "SELECT value FROM settings WHERE key='admin_user' LIMIT 1;" 2>/dev/null)
    fi
    if [[ -z "$current_user" ]]; then
        current_user=$(grep 'admin_user:' "$CONFIG_FILE" | awk '{print $2}' | tr -d '"')
    fi
    info "当前管理员用户名: ${current_user}"

    # 输入新密码
    while true; do
        read -sp "新密码 (≥8位): " NEW_PASS
        echo
        if [[ ${#NEW_PASS} -ge 8 ]]; then
            read -sp "确认密码: " NEW_PASS_CONFIRM
            echo
            if [[ "$NEW_PASS" == "$NEW_PASS_CONFIRM" ]]; then
                break
            else
                warn "两次密码不一致"
            fi
        else
            warn "密码长度至少 8 位"
        fi
    done

    # 直接更新数据库中的密码 hash 并 bump token_version（让历史 JWT 立即失效）
    # 不再 sed config.yaml：旧逻辑只在数据库为空时才把 config 写入 DB，等于"重置无效"
    [[ ! -x "${INSTALL_DIR}/proxy-panel" ]] && error "未找到 ${INSTALL_DIR}/proxy-panel 二进制"
    "${INSTALL_DIR}/proxy-panel" -config "${CONFIG_FILE}" -reset-pass "${NEW_PASS}" || error "重置失败，请查看输出"

    # 可选：同时关闭 2FA（用户丢失 authenticator 时使用）
    local totp_enabled=""
    if [[ -f "$DB_FILE" ]] && command -v sqlite3 &>/dev/null; then
        totp_enabled=$(sqlite3 "$DB_FILE" "SELECT value FROM settings WHERE key='totp_enabled' LIMIT 1;" 2>/dev/null)
    fi
    if [[ "$totp_enabled" == "true" ]]; then
        read -p "检测到已启用 2FA，是否同时关闭 2FA？[y/N]: " disable_2fa
        if [[ "$disable_2fa" =~ ^[Yy]$ ]]; then
            "${INSTALL_DIR}/proxy-panel" -config "${CONFIG_FILE}" -disable-totp || warn "关闭 2FA 失败，请查看输出"
        fi
    fi

    # 重启面板，让正在运行的进程清理任何缓存状态
    systemctl restart ${SERVICE_NAME} 2>/dev/null || warn "面板重启失败，请手动重启"

    echo ""
    info "密码已重置，所有现有登录会话已失效"
}

# ============================================
# disable-2fa 子命令: 应急关闭 2FA（authenticator 设备丢失时）
# ============================================

do_disable_2fa() {
    check_root

    [[ ! -f "$CONFIG_FILE" ]] && error "配置文件不存在: $CONFIG_FILE"
    [[ ! -x "${INSTALL_DIR}/proxy-panel" ]] && error "未找到 ${INSTALL_DIR}/proxy-panel 二进制"

    echo ""
    step "关闭 2FA（两步验证）"
    warn "此操作仅用于 authenticator 设备丢失后的应急解锁"
    read -p "确认关闭 2FA？[y/N]: " confirm
    [[ ! "$confirm" =~ ^[Yy]$ ]] && { info "已取消"; return; }

    "${INSTALL_DIR}/proxy-panel" -config "${CONFIG_FILE}" -disable-totp || error "关闭 2FA 失败，请查看输出"

    systemctl restart ${SERVICE_NAME} 2>/dev/null || warn "面板重启失败，请手动重启"

    echo ""
    info "2FA 已关闭，所有现有登录会话已失效"
}

# ============================================
# backup 子命令: 备份
# ============================================

do_backup() {
    check_root

    [[ ! -d "$INSTALL_DIR" ]] && error "安装目录不存在: $INSTALL_DIR"

    mkdir -p "$BACKUP_DIR"
    local timestamp
    timestamp=$(date +%Y%m%d_%H%M%S)
    local backup_file="${BACKUP_DIR}/proxy-panel-backup-${timestamp}.tar.gz"

    step "创建备份..."

    # 备份配置文件、数据库、证书
    local files_to_backup=()
    [[ -f "$CONFIG_FILE" ]] && files_to_backup+=("config.yaml")
    [[ -d "${INSTALL_DIR}/data" ]] && files_to_backup+=("data")
    [[ -d "${INSTALL_DIR}/certs" ]] && files_to_backup+=("certs")
    [[ -d "${INSTALL_DIR}/kernel" ]] && files_to_backup+=("kernel")

    if [[ ${#files_to_backup[@]} -eq 0 ]]; then
        warn "没有需要备份的文件"
        return 0
    fi

    tar -czf "$backup_file" -C "$INSTALL_DIR" "${files_to_backup[@]}" 2>/dev/null

    local size
    size=$(du -sh "$backup_file" | cut -f1)
    info "备份完成: ${backup_file} (${size})"

    # 清理旧备份 (保留最近 10 个)
    local backup_count
    backup_count=$(ls -1 "${BACKUP_DIR}"/proxy-panel-backup-*.tar.gz 2>/dev/null | wc -l)
    if [[ "$backup_count" -gt 10 ]]; then
        ls -1t "${BACKUP_DIR}"/proxy-panel-backup-*.tar.gz | tail -n +11 | xargs rm -f
        info "已清理旧备份，保留最近 10 个"
    fi
}

# ============================================
# restore 子命令: 从备份恢复
# ============================================

do_restore() {
    check_root

    local backup_file="$1"

    # 如果未指定文件，列出可用备份
    if [[ -z "$backup_file" ]]; then
        echo ""
        echo "可用备份:"
        if [[ -d "$BACKUP_DIR" ]]; then
            local i=1
            local backups=()
            while IFS= read -r f; do
                backups+=("$f")
                local size
                size=$(du -sh "$f" | cut -f1)
                echo "  [${i}] $(basename "$f") (${size})"
                ((i++))
            done < <(ls -1t "${BACKUP_DIR}"/proxy-panel-backup-*.tar.gz 2>/dev/null)

            if [[ ${#backups[@]} -eq 0 ]]; then
                error "没有可用的备份文件"
            fi

            echo ""
            read -p "选择备份编号: " choice
            if [[ "$choice" -ge 1 && "$choice" -le ${#backups[@]} ]] 2>/dev/null; then
                backup_file="${backups[$((choice-1))]}"
            else
                error "无效的选择"
            fi
        else
            error "备份目录不存在: $BACKUP_DIR"
        fi
    fi

    [[ ! -f "$backup_file" ]] && error "备份文件不存在: $backup_file"

    echo ""
    warn "恢复将覆盖当前配置和数据"
    if ! confirm "确认恢复?"; then
        info "取消恢复"
        exit 0
    fi

    step "恢复备份: $(basename "$backup_file")"

    # 停止服务
    systemctl stop ${SERVICE_NAME} 2>/dev/null || true
    systemctl stop ${XRAY_SERVICE} 2>/dev/null || true

    # 解压恢复
    mkdir -p "$INSTALL_DIR"
    tar -xzf "$backup_file" -C "$INSTALL_DIR"

    # 修复权限
    chmod 600 "$CONFIG_FILE" 2>/dev/null || true

    # 重启服务
    systemctl start ${XRAY_SERVICE} 2>/dev/null || true
    systemctl start ${SERVICE_NAME} 2>/dev/null || true

    echo ""
    info "备份恢复完成，服务已重启"
}

# ============================================
# 修改面板端口子命令: proxy-panel port [新端口]
# ============================================

do_port() {
    check_root
    [[ -f "$CONFIG_FILE" ]] || error "未找到配置文件 ${CONFIG_FILE}，请先安装"

    # 读取当前端口（复用 show_context_hint 的解析方式）
    local cur_port
    cur_port=$(awk '/^server:/{flag=1;next} /^[a-z]/{flag=0} flag && /port:/{print $2; exit}' "$CONFIG_FILE" 2>/dev/null)
    : "${cur_port:=8080}"

    # 新端口：优先取命令行参数，否则交互输入
    local new_port="${2:-}"
    if [[ -z "$new_port" ]]; then
        echo "当前面板端口: ${cur_port}"
        read -p "新端口: " new_port
    fi

    # 校验（与 interactive_config 一致）
    if ! [[ "$new_port" =~ ^[0-9]+$ ]] || [[ "$new_port" -lt 1 || "$new_port" -gt 65535 ]]; then
        error "无效的端口号: ${new_port}"
    fi
    if [[ "$new_port" == "$cur_port" ]]; then
        info "端口未变化 (${cur_port})，无需修改"
        return 0
    fi

    # 仅修改 server.port 这一行
    sed -i "s|^  port:.*|  port: ${new_port}|" "$CONFIG_FILE"
    info "config.yaml 端口已更新: ${cur_port} → ${new_port}"

    # 更换管理端口不自动扩大公网防火墙开放范围。

    # 重启面板生效
    systemctl restart "${SERVICE_NAME}" 2>/dev/null && info "ProxyPanel 已重启" \
        || warn "ProxyPanel 重启失败，请执行 proxy-panel status 查看"

    echo ""
    info "✅ 面板端口已改为 ${new_port}"
    warn "请同步更新 SSH 隧道或受保护的反代；旧端口 ${cur_port} 如不再使用可自行回收"
    warn "若用 Cloudflare 橙色云代理，HTTPS 仅支持 443/2053/2083/2087/2096/8443 端口"
}

# ============================================
# 显示帮助
# ============================================

show_help() {
    echo ""
    echo -e "${BLUE}ProxyPanel 管理脚本${NC}"
    echo ""
    echo "用法: proxy-panel <命令> [参数]"
    echo ""
    echo -e "${GREEN}[安装]${NC}"
    echo "  install            完整安装 ProxyPanel"
    echo "  update             升级 (预检、保留配置、失败回退)"
    echo "  rollback-update <目录>  从 update-* 备份回退程序和数据"
    echo "  uninstall          卸载 (可选保留数据)"
    echo ""
    echo -e "${GREEN}[运维]${NC}"
    echo "  status             查看服务状态"
    echo "  restart            重启所有服务"
    echo "  port [新端口]      修改面板端口 (改 config + 重启)"
    echo "  logs [svc] [行数]  查看日志"
    echo "  backup             备份配置和数据"
    echo "  restore [文件]     从备份恢复"
    echo ""
    echo -e "${GREEN}[安全]${NC}"
    echo "  reset-pwd          重置管理员密码"
    echo "  disable-2fa        应急关闭 2FA (authenticator 设备丢失时)"
    echo "  cert <子命令>      证书管理 (setup|status|renew)"
    echo "  website setup [域名]  安装 Nginx、部署 443 网站及 HTTPUpgrade 反代"
    echo "  website status    查看托管网站与证书续期监听状态"
    echo "  firewall <子命令>  防火墙管理 (enable|disable|status)"
    echo ""
    echo -e "${GREEN}[其它]${NC}"
    echo "  help               显示此帮助"
    echo ""
}

# 根据安装状态给出上下文建议；主入口无参数时调用
show_context_hint() {
    if [[ -f "$CONFIG_FILE" ]] && systemctl is-active --quiet "$SERVICE_NAME" 2>/dev/null; then
        local port
        port=$(awk '/^server:/{flag=1;next} /^[a-z]/{flag=0} flag && /port:/{print $2; exit}' "$CONFIG_FILE" 2>/dev/null)
        : "${port:=8080}"
        echo -e "${GREEN}当前状态: 已安装并运行 (端口 ${port})${NC}"
        echo -e "常用操作: ${YELLOW}proxy-panel status${NC} | ${YELLOW}proxy-panel logs${NC} | ${YELLOW}proxy-panel restart${NC}"
    elif [[ -f "$CONFIG_FILE" ]]; then
        echo -e "${YELLOW}当前状态: 已安装但服务未运行${NC}"
        echo -e "建议: ${YELLOW}proxy-panel status${NC} 查看状态，或 ${YELLOW}proxy-panel restart${NC} 重启服务"
    else
        echo -e "${YELLOW}当前状态: 尚未安装${NC}"
        echo -e "建议: 首次安装请执行 ${YELLOW}proxy-panel install${NC}"
    fi
    echo ""
}

# ============================================
# 主入口
# ============================================

main() {
    case "${1:-}" in
        install)    do_install ;;
        update)     do_update ;;
        rollback-update) do_rollback_update "${2:-}" ;;
        uninstall)  do_uninstall ;;
        status)     do_status ;;
        restart)    do_restart ;;
        port)       do_port "$@" ;;
        logs)       do_logs "$@" ;;
        reset-pwd)  do_reset_pwd ;;
        disable-2fa) do_disable_2fa ;;
        backup)     do_backup ;;
        restore)    do_restore "$2" ;;
        cert)       do_cert "$@" ;;
        website)    do_website "$@" ;;
        firewall)   do_firewall "$@" ;;
        help|--help|-h)
            show_help ;;
        "")
            show_help
            show_context_hint
            ;;
        *)
            error "未知命令: $1\n用法: proxy-panel {install|update|uninstall|status|restart|logs|reset-pwd|disable-2fa|backup|restore|cert|firewall|help}"
            ;;
    esac
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then main "$@"; fi
