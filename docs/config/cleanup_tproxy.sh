#!/bin/bash
# mihomo tproxy iptables 清理脚本
# 当 mihomo 意外终止导致 iptables 规则残留时，运行此脚本恢复网络
# 用法: sudo bash cleanup_tproxy.sh [网卡名] [DNS端口]
# 示例: sudo bash cleanup_tproxy.sh ens33 1053

set -e

IFACE="${1:-ens33}"
DNS_PORT="${2:-1053}"
FWMARK="0x2d0"
TABLE="0x2d0"

echo "=== 清理 mihomo tproxy iptables 规则 ==="
echo "网卡: $IFACE"
echo "DNS端口: $DNS_PORT"
echo ""

# 检查是否有残留规则
if ! iptables -t mangle -L mihomo_divert &>/dev/null; then
    echo "未发现 mihomo iptables 规则，无需清理"
    exit 0
fi

echo "[1/6] 清理策略路由 ..."
ip -f inet rule del fwmark $FWMARK lookup $TABLE 2>/dev/null || true
ip -f inet route del local default dev $IFACE table $TABLE 2>/dev/null || true

echo "[2/6] 清理 FORWARD 链 ..."
if [ "$IFACE" != "lo" ]; then
    iptables -t filter -D FORWARD -i $IFACE ! -o $IFACE -j ACCEPT 2>/dev/null || true
    iptables -t filter -D FORWARD -i $IFACE -o $IFACE -j ACCEPT 2>/dev/null || true
    iptables -t filter -D FORWARD -o $IFACE -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT 2>/dev/null || true
    iptables -t filter -D FORWARD -o $IFACE -j ACCEPT 2>/dev/null || true
fi

echo "[3/6] 清理 PREROUTING 链 ..."
iptables -t nat -D PREROUTING ! -s 172.17.0.0/16 ! -d 127.0.0.0/8 -p tcp --dport 53 -j REDIRECT --to $DNS_PORT 2>/dev/null || true
iptables -t nat -D PREROUTING ! -s 172.17.0.0/16 ! -d 127.0.0.0/8 -p udp --dport 53 -j REDIRECT --to $DNS_PORT 2>/dev/null || true
iptables -t mangle -D PREROUTING -j mihomo_prerouting 2>/dev/null || true

echo "[4/6] 清理 POSTROUTING 链 ..."
if [ "$IFACE" != "lo" ]; then
    iptables -t nat -D POSTROUTING -o $IFACE -m addrtype ! --src-type LOCAL -j MASQUERADE 2>/dev/null || true
fi

echo "[5/6] 清理 OUTPUT 链 ..."
iptables -t mangle -D OUTPUT -o $IFACE -j mihomo_output 2>/dev/null || true
iptables -t nat -D OUTPUT -p tcp --dport 53 -j mihomo_dns_output 2>/dev/null || true
iptables -t nat -D OUTPUT -p udp --dport 53 -j mihomo_dns_output 2>/dev/null || true

echo "[6/6] 删除自定义链 ..."
iptables -t mangle -F mihomo_prerouting 2>/dev/null || true
iptables -t mangle -X mihomo_prerouting 2>/dev/null || true
iptables -t mangle -F mihomo_divert 2>/dev/null || true
iptables -t mangle -X mihomo_divert 2>/dev/null || true
iptables -t mangle -F mihomo_output 2>/dev/null || true
iptables -t mangle -X mihomo_output 2>/dev/null || true
iptables -t nat -F mihomo_dns_output 2>/dev/null || true
iptables -t nat -X mihomo_dns_output 2>/dev/null || true

echo ""
echo "=== 清理完成 ==="
echo "验证网络: ping -c 3 8.8.8.8"
