#!/bin/bash

urls=(
    "https://registry.cn-hangzhou.aliyuncs.com"
    "https://docker.m.daocloud.io"
    "https://mirror.iscas.ac.cn"
    "http://mirrors.ustc.edu.cn/"
    "http://mirrors.sohu.com/"
    "https://docker.xuanyuan.me"
    "https://docker.1ms.run"
    "https://atomhub.openatom.cn"
    "https://ccr.ccs.tencentyun.com"
    "https://hub.rat.dev"
    "https://xuanyuan.cloud"
)

echo "开始检测 URL 可用性..."
echo "=========================================="

for url in "${urls[@]}"; do
    # 使用 curl 检测，超时 5 秒，仅检查 HTTP 状态码
    status=$(curl -o /dev/null -s -w "%{http_code}" --connect-timeout 5 --max-time 10 "$url")
    if [[ "$status" -ge 200 && "$status" -lt 400 ]]; then
        echo "✅ $url - 有效 (HTTP $status)"
    else
        echo "❌ $url - 失效 (HTTP $status)"
    fi
done

echo "=========================================="
echo "检测完成！"