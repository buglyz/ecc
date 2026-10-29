# Quanta ControlCenter 驱动（`\\.\CCDRV1`）IOCTL 协议

> 目标：`D:\桌面\ControlCenter\ControlCenter64.sys`
> SHA-256：`29eff643548103a94873608d88be2ea0659556aa3288c591538d5f9a198de6f9`
> 签名：QUANTA COMPUTER INC. / GlobalSign EV Code Signing（2026-07-05 到期）
> 设备名：`\Device\CCDRV1` → `\DosDevices\CCDRV1` → 用户态 `\\.\CCDRV1`
> 服务名：`ControlCenter`

## 1. 当前机器上的加载状态

```text
sc query ControlCenter
STATE : 1 STOPPED

sc start ControlCenter
[SC] StartService 失败 577:
Windows 无法验证此文件的数字签名……
```

系统状态：

```text
VirtualizationBasedSecurityStatus    : 2   (Running)
SecurityServicesRunning              : {2} (HVCI)
CodeIntegrityPolicyEnforcementStatus : 2   (Enforced)
```

结论：**HVCI / 驱动完整性策略拒绝加载该 OEM 驱动**。除非关闭 HVCI、启用测试签名并自行重签驱动，否则 `\\.\CCDRV1` 在本机不可用。

## 2. IOCTL 总表

驱动入口 `FUN_000110e4` 根据 `IoControlCode` 分发。以下为反汇编还原结果。

| IOCTL | 功能 | 输入 | 输出 |
|-------|------|------|------|
| `0x9C402000` | 返回 8 | — | `DWORD 8` |
| `0x9C402004` | 打开计数 | — | `DWORD` |
| `0x9C402084` | `rdmsr` | `DWORD msr` | `QWORD` |
| `0x9C402088` | `wrmsr` | `DWORD msr` + `QWORD value` | — |
| `0x9C40208C` | `rdpmc` | `DWORD counter` | `QWORD` |
| `0x9C4060CC` | 读端口 1 字节 | `DWORD port` | 缓冲区首字节 |
| `0x9C4060D0` | 读端口 2 字节 | `DWORD port` | 缓冲区首 2 字节 |
| `0x9C4060D4` | 读端口 4 字节 | `DWORD port` | 缓冲区首 4 字节 |
| `0x9C40A0D8` | 写端口 1 字节 | `DWORD port` + `BYTE value` @0x04 | — |
| `0x9C40A0DC` | 写端口 2 字节 | `DWORD port` + `WORD value` @0x04 | — |
| `0x9C40A0E0` | 写端口 4 字节 | `DWORD port` + `DWORD value` @0x04 | — |
| `0x9C406104` | 物理内存读 | `{u64 phys; u32 width; u32 count}` | 数据 |
| `0x9C40A108` | 物理内存写 | `{u64 phys; u32 width; u32 count}` + 数据 @0x10 | — |
| `0x9C406144` | PCI 配置读 | `{u32 addr; u32 offset; ...}` | 数据 |
| `0x9C40A148` | PCI 配置写 | 同上 + 数据 | — |
| `0x9C402090` | **HLT（不要调用）** | — | — |
| `0xA2AB208C` | 存活探测 | 长度 ≥ 4 | `WORD 1, WORD 0` |

### 端口读的缓冲区语义

驱动把 `SystemBuffer` 同时当作输入和输出：

```c
port = *(DWORD *)SystemBuffer;
value = in(port);              // 1/2/4 字节
*(BYTE/WORD/DWORD *)SystemBuffer = value;
IoStatus.Information = InputBufferLength;
```

因此用户态应传入至少 4 字节的输入缓冲，并从返回缓冲的首字节/首字/首双字读取结果。

### 端口写的缓冲区语义

```c
port  = *(DWORD *)SystemBuffer;      // offset 0
value = *(BYTE/WORD/DWORD *)(SystemBuffer + 4);
out(port, value);
IoStatus.Information = InputBufferLength;
```

## 3. EC 访问协议

EC 通过 ACPI 标准端口访问：

| 端口 | 用途 |
|------|------|
| `0x66` | 命令/状态寄存器 |
| `0x62` | 数据寄存器 |

状态位：

- `bit 0`：OBF（Output Buffer Full）
- `bit 1`：IBF（Input Buffer Full）

读寄存器：

```text
1. 等 0x66 的 IBF(bit1) = 0
2. 写 0x66 = 0x80          // READ_EC
3. 等 IBF = 0
4. 写 0x62 = register
5. 等 0x66 的 OBF(bit0) = 1
6. 读 0x62
```

写寄存器：

```text
1. 等 IBF = 0
2. 写 0x66 = 0x81          // WRITE_EC
3. 等 IBF = 0
4. 写 0x62 = register
5. 等 IBF = 0
6. 写 0x62 = value
```

本机用 `ec-probe` 只读对照得到的值：

```text
0x2C = 0x4C (76)
0x2D = 0x4C (76)
0xB0/0xB1 = 0x10F3 = 4339 RPM
0xB2/0xB3 = 0x106D = 4205 RPM
```

## 4. 如果将来要启用该驱动

需要满足以下任一条件：

1. 关闭 HVCI / 内存完整性，并启用测试签名 + 自签驱动；
2. 使用 Microsoft 接受的 WHQL/EV 重签版本；
3. 换一台未启用 HVCI 的机器。

不建议为了风扇控制关闭 HVCI。当前可用路径仍是 ECC 原有的 **EC + WinRing0（ec-probe）** 后端。
