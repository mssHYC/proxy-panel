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

confirm() {
    local msg="${1:-确认继续?}"
    read -p "${msg} [y/N]: " answer
    [[ "$answer" =~ ^[Yy]$ ]]
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
    if [[ ! -f ~/.acme.sh/acme.sh ]]; then
        info "安装 acme.sh..."
        curl -s https://get.acme.sh | sh -s email=admin@"${DOMAIN}" 2>/dev/null || {
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
    echo "  [6] 不使用 TLS (纯 IP 直连)"
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
            echo "  请确保: 1) 域名 A 记录已指向本机 IP  2) 80 端口未被占用"
            echo ""

            ~/.acme.sh/acme.sh --issue -d "$DOMAIN" --standalone --keylength ec-256 || {
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
            read -p "Cloudflare API Token (Zone.DNS 权限): " CF_TOKEN
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
                ~/.acme.sh/acme.sh --issue -d "$MAIN_DOMAIN" -d "$DOMAIN" --dns dns_cf --keylength ec-256 || {
                    warn "证书申请失败，请检查 API Token 权限和域名是否在 Cloudflare"
                    return 0
                }
                DOMAIN="$MAIN_DOMAIN"
            else
                ~/.acme.sh/acme.sh --issue -d "$DOMAIN" --dns dns_cf --keylength ec-256 || {
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
            read -p "DNSPod API ID: " DP_ID
            read -p "DNSPod API Token: " DP_KEY
            [[ -z "$DP_ID" || -z "$DP_KEY" ]] && error "API ID 和 Token 不能为空"

            install_acme || {
                warn "跳过证书申请"
                return 0
            }

            info "正在通过 DNSPod DNS API 申请证书..."
            export DP_Id="$DP_ID"
            export DP_Key="$DP_KEY"

            ~/.acme.sh/acme.sh --issue -d "$DOMAIN" --dns dns_dp --keylength ec-256 || {
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
            read -p "阿里云 AccessKey ID: " ALI_KEY
            read -p "阿里云 AccessKey Secret: " ALI_SECRET
            [[ -z "$ALI_KEY" || -z "$ALI_SECRET" ]] && error "AccessKey 不能为空"

            install_acme || {
                warn "跳过证书申请"
                return 0
            }

            info "正在通过阿里云 DNS API 申请证书..."
            export Ali_Key="$ALI_KEY"
            export Ali_Secret="$ALI_SECRET"

            ~/.acme.sh/acme.sh --issue -d "$DOMAIN" --dns dns_ali --keylength ec-256 || {
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
            info "不使用 TLS，面板将以 HTTP 模式运行"
            ;;
        *)
            error "无效的选项: $TLS_MODE"
            ;;
    esac
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
        # 匹配 cert 行之后紧跟的 key 行
        sed -i "/^  cert:/{ n; s|^  key:.*|  key: \"${KEY_PATH}\"|; }" "$CONFIG_FILE"
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
  port: ${PANEL_PORT}
  tls: ${TLS_ENABLED}
  cert: "${CERT_PATH}"
  key: "${KEY_PATH}"

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
  "experimental": {
    "v2ray_api": {
      "listen": "127.0.0.1:9090",
      "stats": {
        "enabled": true,
        "inbounds": [],
        "outbounds": [],
        "users": []
      }
    }
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
Restart=on-failure
RestartSec=5
LimitNOFILE=65535
StandardOutput=journal
StandardError=journal

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
    if command -v ufw &>/dev/null; then
        ufw allow "${PANEL_PORT}/tcp" >/dev/null 2>&1
        info "ufw 已放行端口 ${PANEL_PORT}"
    elif command -v firewall-cmd &>/dev/null; then
        firewall-cmd --permanent --add-port="${PANEL_PORT}/tcp" >/dev/null 2>&1
        firewall-cmd --reload >/dev/null 2>&1
        info "firewalld 已放行端口 ${PANEL_PORT}"
    else
        warn "未检测到防火墙工具，请手动放行端口 ${PANEL_PORT}"
    fi
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
    echo "  访问地址: ${protocol}://${access_host}:${PANEL_PORT}"
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

    # 检查是否已安装
    if [[ -f "${INSTALL_DIR}/proxy-panel" ]]; then
        warn "检测到已安装 ProxyPanel"
        if confirm "是否覆盖安装? (配置和数据将保留)"; then
            stop_all_services
            info "开始覆盖安装..."
        else
            info "取消安装"
            exit 0
        fi
    fi

    install_deps
    interactive_config
    generate_config
    generate_default_xray_config
    generate_default_singbox_config
    download_xray
    download_singbox
    download_panel
    install_cli
    setup_systemd
    setup_firewall
    start_services
    print_summary
}

# ============================================
# update 子命令: 升级 (保留配置和数据)
# ============================================

do_update() {
    check_root
    detect_os
    detect_arch

    [[ ! -f "$CONFIG_FILE" ]] && error "未检测到已安装的 ProxyPanel，请先执行 install"

    step "升级 ProxyPanel..."

    # 先备份
    info "升级前自动备份..."
    do_backup

    # 停止所有服务
    stop_all_services

    # 下载新版本
    download_panel

    # 可选: 更新 Xray
    if confirm "是否同时更新 Xray?"; then
        download_xray
    fi

    # 可选: 更新 Sing-box
    if confirm "是否同时更新 Sing-box?"; then
        download_singbox
    fi

    # 重新加载并启动
    systemctl daemon-reload
    start_services

    echo ""
    info "升级完成! 配置和数据已保留。"
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
    local current_user
    current_user=$(grep 'admin_user:' "$CONFIG_FILE" | awk '{print $2}' | tr -d '"')
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

    # 更新配置文件中的密码
    sed -i "s|admin_pass:.*|admin_pass: \"${NEW_PASS}\"|" "$CONFIG_FILE"

    # 重启面板使新密码生效
    systemctl restart ${SERVICE_NAME} 2>/dev/null || warn "面板重启失败，请手动重启"

    echo ""
    info "密码已重置，面板已重启"
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
# 显示帮助
# ============================================

show_help() {
    echo ""
    echo "ProxyPanel 管理脚本"
    echo ""
    echo "用法: proxy-panel <命令> [参数]"
    echo ""
    echo "命令:"
    echo "  install     完整安装 ProxyPanel"
    echo "  update      升级 (保留配置和数据)"
    echo "  uninstall   卸载 (可选保留数据)"
    echo "  status      查看服务状态"
    echo "  restart     重启所有服务"
    echo "  logs        查看日志 (可选: logs [服务名] [行数])"
    echo "  reset-pwd   重置管理员密码"
    echo "  backup      备份配置和数据"
    echo "  restore     从备份恢复 (可选: restore <文件路径>)"
    echo "  cert        证书管理 (cert setup|status|renew)"
    echo "  help        显示此帮助"
    echo ""
}

# ============================================
# 主入口
# ============================================

main() {
    case "${1:-}" in
        install)    do_install ;;
        update)     do_update ;;
        uninstall)  do_uninstall ;;
        status)     do_status ;;
        restart)    do_restart ;;
        logs)       do_logs "$@" ;;
        reset-pwd)  do_reset_pwd ;;
        backup)     do_backup ;;
        restore)    do_restore "$2" ;;
        cert)       do_cert "$@" ;;
        help|--help|-h)
            show_help ;;
        "")
            show_help
            echo -e "${YELLOW}提示: 首次安装请执行: proxy-panel install${NC}"
            echo ""
            ;;
        *)
            error "未知命令: $1\n用法: proxy-panel {install|update|uninstall|status|restart|logs|reset-pwd|backup|restore|cert|help}"
            ;;
    esac
}

main "$@"
