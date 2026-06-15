#!/bin/bash
echo "测试常见的风扇 RPM 寄存器位置..."
echo "注意：如果能读到有意义的值（不是 0 且在合理范围），可能就是 RPM 寄存器"
echo ""

# 常见的 RPM 寄存器位置
declare -a candidates=(
    "0xCA,0xCB"  # Fan1 RPM (某些 ITE EC)
    "0xCC,0xCD"  # Fan2 RPM (某些 ITE EC) 
    "0xC0,0xC1"  # Fan1 RPM (另一种布局)
    "0xC2,0xC3"  # Fan2 RPM
    "0xD0,0xD1"  # Fan1 RPM (已测试，但再试一次)
    "0xD2,0xD3"  # Fan2 RPM
    "0xE0,0xE1"  # Fan1 RPM (某些型号)
    "0xE2,0xE3"  # Fan2 RPM
    "0xFC,0xFD"  # Fan RPM (某些神舟型号)
)

for pair in "${candidates[@]}"; do
    IFS=',' read -ra ADDR <<< "$pair"
    low=${ADDR[0]}
    high=${ADDR[1]}
    
    low_val=$(./ec-probe.exe read $low 2>&1 | awk '{print $1}')
    high_val=$(./ec-probe.exe read $high 2>&1 | awk '{print $1}')
    
    # 计算 RPM (假设是小端序)
    if [[ "$low_val" =~ ^[0-9]+$ ]] && [[ "$high_val" =~ ^[0-9]+$ ]]; then
        rpm=$((high_val * 256 + low_val))
        echo "$low,$high: Low=$low_val High=$high_val => RPM=$rpm"
    fi
done
