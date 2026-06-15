#!/bin/bash
# 扫描哪些寄存器在变化（可能是 RPM）

echo "第一次扫描（基准）..."
for i in {0..255}; do
  hex=$(printf "0x%02X" $i)
  val=$(./ec-probe.exe read $hex 2>&1 | awk '{print $1}')
  echo "$hex $val"
done > scan1.txt

sleep 2

echo "第二次扫描..."
for i in {0..255}; do
  hex=$(printf "0x%02X" $i)
  val=$(./ec-probe.exe read $hex 2>&1 | awk '{print $1}')
  echo "$hex $val"
done > scan2.txt

echo "对比变化的寄存器："
diff scan1.txt scan2.txt | grep "^[<>]" | head -20
