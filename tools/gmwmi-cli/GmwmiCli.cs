using System;
using System.Management;
using System.Text;

// GmwmiCli：NL5 / Gaming WMI 的薄封装。
//
// 只负责把 32 字节缓冲区写入 root\wmi:RW_GMWMI.BufferBytes 并读回；
// 命令字、字段偏移等协议逻辑全部在 Go 侧（internal/gmwmi），这样协议
// 可以单元测试，C# 侧保持最小、最容易审计。
internal static class Program
{
    private const string WmiNamespace = @"root\wmi";
    private const string WmiClass = "RW_GMWMI";
    private const int BufferSize = 32;

    private static byte[] ReadBuffer()
    {
        ManagementScope scope = new ManagementScope(WmiNamespace);
        SelectQuery query = new SelectQuery(WmiClass);
        using (ManagementObjectSearcher searcher = new ManagementObjectSearcher(scope, query))
        {
            foreach (ManagementObject mo in searcher.Get())
            {
                object value = mo["BufferBytes"];
                byte[] bytes = value as byte[];
                if (bytes == null || bytes.Length != BufferSize)
                {
                    throw new Exception("BufferBytes is missing or not " + BufferSize + " bytes");
                }

                byte[] copy = new byte[BufferSize];
                Buffer.BlockCopy(bytes, 0, copy, 0, BufferSize);
                return copy;
            }
        }

        throw new Exception("RW_GMWMI instance not found");
    }

    private static void WriteBuffer(byte[] buffer)
    {
        if (buffer == null || buffer.Length != BufferSize)
        {
            throw new Exception("buffer must be exactly " + BufferSize + " bytes");
        }

        ManagementScope scope = new ManagementScope(WmiNamespace);
        SelectQuery query = new SelectQuery(WmiClass);
        using (ManagementObjectSearcher searcher = new ManagementObjectSearcher(scope, query))
        {
            foreach (ManagementObject mo in searcher.Get())
            {
                mo.SetPropertyValue("BufferBytes", buffer);
                mo.Put();
                return;
            }
        }

        throw new Exception("RW_GMWMI instance not found");
    }

    private static string ToHex(byte[] data)
    {
        StringBuilder sb = new StringBuilder(data.Length * 2);
        for (int i = 0; i < data.Length; i++)
        {
            sb.Append(data[i].ToString("x2"));
        }
        return sb.ToString();
    }

    private static byte[] FromHex(string hex)
    {
        if (hex == null)
        {
            throw new Exception("hex input is null");
        }

        hex = hex.Trim();
        if (hex.Length != BufferSize * 2)
        {
            throw new Exception("hex input must be exactly " + (BufferSize * 2) + " characters");
        }

        byte[] data = new byte[BufferSize];
        for (int i = 0; i < BufferSize; i++)
        {
            data[i] = Convert.ToByte(hex.Substring(i * 2, 2), 16);
        }
        return data;
    }

    private static int ShowInfo()
    {
        ManagementScope scope = new ManagementScope(WmiNamespace);
        SelectQuery query = new SelectQuery(WmiClass);
        using (ManagementObjectSearcher searcher = new ManagementObjectSearcher(scope, query))
        {
            foreach (ManagementObject mo in searcher.Get())
            {
                object instanceName = mo["InstanceName"];
                object active = mo["Active"];
                object buffer = mo["BufferBytes"];
                Console.WriteLine("InstanceName=" + (instanceName == null ? "" : instanceName.ToString()));
                Console.WriteLine("Active=" + (active == null ? "" : active.ToString()));
                Console.WriteLine("BufferBytes=" + (buffer is byte[] ? ToHex((byte[])buffer) : ""));
                return 0;
            }
        }

        Console.Error.WriteLine("error: RW_GMWMI instance not found");
        return 1;
    }

    private static int Main(string[] args)
    {
        try
        {
            if (args.Length == 1 && args[0] == "read")
            {
                Console.WriteLine(ToHex(ReadBuffer()));
                return 0;
            }

            if (args.Length == 2 && args[0] == "write")
            {
                WriteBuffer(FromHex(args[1]));
                return 0;
            }

            if (args.Length == 2 && args[0] == "xfer")
            {
                WriteBuffer(FromHex(args[1]));
                Console.WriteLine(ToHex(ReadBuffer()));
                return 0;
            }

            if (args.Length == 1 && args[0] == "info")
            {
                return ShowInfo();
            }

            Console.Error.WriteLine("usage: GmwmiCli read | write <64hex> | xfer <64hex> | info");
            return 2;
        }
        catch (Exception ex)
        {
            Console.Error.WriteLine("error: " + ex.Message);
            return 1;
        }
    }
}
