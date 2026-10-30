; ECC Fan Controller - Inno Setup 安装脚本
; 需要 Inno Setup 6.3+。
; 构建：ISCC.exe /DAppVersion=<版本> installer\ecc.iss
; 产物：dist\ECC-Setup-<版本>.exe（单文件，内含 ecc.exe 与 assets/）

#ifndef AppVersion
  #define AppVersion "0.0.0-dev"
#endif

; CI 通过 /DSourceRoot=<仓库绝对路径> 传入，避免相对路径解析歧义。
#ifndef SourceRoot
  #define SourceRoot ".."
#endif

#define AppName "ECC Fan Controller"
#define AppExeName "ecc.exe"
#define AppPublisher "buglyz"
; 升级/卸载身份必须跨版本保持不变。
#define AppId "{{8C1F0B47-6A24-4F0F-9C39-7D3B9E1A2F10}"
#define MutexName "Global\ECC-FanController-SingleInstance"

[Setup]
AppId={#AppId}
AppName={#AppName}
AppVersion={#AppVersion}
AppPublisher={#AppPublisher}
DefaultDirName={autopf}\ECC
DefaultGroupName={#AppName}
DisableProgramGroupPage=yes
; 升级行为：固定 AppId 让 Inno 识别旧版本；沿用旧目录/任务，
; 升级时跳过目录页，并在复制前清理旧的 assets，避免残留文件。
UsePreviousAppDir=yes
UsePreviousTasks=yes
DisableDirPage=auto
PrivilegesRequired=admin
OutputDir=..\dist
OutputBaseFilename=ECC-Setup-{#AppVersion}
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
MinVersion=10.0
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
UninstallDisplayIcon={app}\{#AppExeName}
CloseApplications=no
SetupLogging=yes

[Languages]
Name: "chinesesimplified"; MessagesFile: "{#SourceRoot}\installer\languages\ChineseSimplified.isl"

[Tasks]
Name: "desktopicon"; Description: "创建桌面快捷方式"; GroupDescription: "附加任务:"

[Files]
Source: "{#SourceRoot}\{#AppExeName}"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#SourceRoot}\assets\*"; DestDir: "{app}\assets"; Flags: ignoreversion recursesubdirs createallsubdirs
Source: "{#SourceRoot}\README.md"; DestDir: "{app}"; Flags: ignoreversion

[InstallDelete]
; 覆盖升级时先清空旧 assets，避免旧版本遗留的 DLL/工具继续存在。
Type: filesandordirs; Name: "{app}\assets"

[Icons]
Name: "{autodesktop}\{#AppName}"; Filename: "{app}\{#AppExeName}"; WorkingDir: "{app}"; Tasks: desktopicon
Name: "{group}\{#AppName}"; Filename: "{app}\{#AppExeName}"; WorkingDir: "{app}"

[Run]
Filename: "{app}\{#AppExeName}"; Description: "立即启动 {#AppName}"; Flags: nowait postinstall skipifsilent

[Code]
const
  EVENT_MODIFY_STATE = $0002;

// PawnIO 是默认 EC 后端。安装包内带有官方 2.2.0 安装器，已安装时跳过。
function NeedsPawnIO: Boolean;
begin
  Result := not RegKeyExists(HKLM, 'SYSTEM\CurrentControlSet\Services\PawnIO');
end;

procedure InstallPawnIO;
var
  ResultCode: Integer;
  SetupPath: String;
begin
  if not NeedsPawnIO then
    exit;
  SetupPath := ExpandConstant('{app}\assets\pawnio\PawnIO_setup.exe');
  if not FileExists(SetupPath) then
  begin
    Log('PawnIO setup file is missing: ' + SetupPath);
    SuppressibleMsgBox('安装包缺少 PawnIO 安装器：' + SetupPath, mbError, MB_OK, IDOK);
    exit;
  end;
  if not Exec(SetupPath, '-install -silent', '', SW_HIDE, ewWaitUntilTerminated, ResultCode) then
  begin
    Log('PawnIO setup failed to start');
    SuppressibleMsgBox('无法启动 PawnIO 安装器。ECC 将回退到 ec-probe。', mbInformation, MB_OK, IDOK);
    exit;
  end;
  // 以服务是否出现为准，避免把安装器的非零退出码误判为失败。
  Sleep(1000);
  if NeedsPawnIO then
  begin
    Log('PawnIO setup finished but the service is still missing; exit code ' + IntToStr(ResultCode));
    SuppressibleMsgBox('PawnIO 安装后未检测到服务，ECC 将回退到 ec-probe。', mbInformation, MB_OK, IDOK);
  end
  else
    Log('PawnIO service installed successfully');
end;

function EccOpenEventW(dwDesiredAccess: DWORD; bInheritHandle: BOOL; lpName: String): THandle;
  external 'OpenEventW@kernel32.dll stdcall';
function EccSetEvent(hEvent: THandle): BOOL;
  external 'SetEvent@kernel32.dll stdcall';
function EccCloseHandle(hObject: THandle): BOOL;
  external 'CloseHandle@kernel32.dll stdcall';

// 通知运行中的实例走正常退出流程（写 0xFF 释放 EC 控制）。
function SignalRunningInstance: Boolean;
var
  H: THandle;
begin
  Result := True;
  H := EccOpenEventW(EVENT_MODIFY_STATE, False, 'Global\ECC-FanController-Shutdown');
  if H = 0 then
    exit; // 没有运行中的实例
  Result := EccSetEvent(H);
  EccCloseHandle(H);
end;

function WaitForInstanceExit(TimeoutMs: Integer): Boolean;
var
  Elapsed: Integer;
begin
  Elapsed := 0;
  while (Elapsed < TimeoutMs) and CheckForMutexes('{#MutexName}') do
  begin
    Sleep(250);
    Elapsed := Elapsed + 250;
  end;
  Result := not CheckForMutexes('{#MutexName}');
end;

// 优雅退出优先；超时才按进程名兜底强杀。单实例保护下同名进程只可能是 ECC。
function StopRunningInstance: Boolean;
var
  ResultCode: Integer;
begin
  SignalRunningInstance;
  Result := WaitForInstanceExit(8000);
  if not Result then
  begin
    Exec(ExpandConstant('{sys}\taskkill.exe'), '/F /IM ecc.exe', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
    Sleep(1000);
    Result := not CheckForMutexes('{#MutexName}');
  end;
end;

procedure MigrateScheduledTask;
var
  ResultCode: Integer;
  Params: String;
begin
  Exec(ExpandConstant('{sys}\schtasks.exe'), '/Query /TN "风扇控制"', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  if ResultCode = 0 then
  begin
    Params := '/Change /TN "风扇控制" /TR "' + ExpandConstant('{app}\ecc.exe') + '"';
    Exec(ExpandConstant('{sys}\schtasks.exe'), Params, '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  end;
end;

procedure RemoveScheduledTask;
var
  ResultCode: Integer;
begin
  Exec(ExpandConstant('{sys}\schtasks.exe'), '/Query /TN "风扇控制"', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  if ResultCode = 0 then
    Exec(ExpandConstant('{sys}\schtasks.exe'), '/Delete /TN "风扇控制" /F', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
end;

procedure DeleteConfigAndLogs;
begin
  DelTree(ExpandConstant('{localappdata}\FanController'), True, True, True);
  DelTree(ExpandConstant('{localappdata}\ecc'), True, True, True);
end;

function IsSilentUninstall: Boolean;
var
  I: Integer;
begin
  Result := False;
  for I := 1 to ParamCount do
    if (CompareText(ParamStr(I), '/SILENT') = 0) or
       (CompareText(ParamStr(I), '/VERYSILENT') = 0) then
    begin
      Result := True;
      Exit;
    end;
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep = ssInstall then
    StopRunningInstance
  else if CurStep = ssPostInstall then
  begin
    MigrateScheduledTask;
    InstallPawnIO;
  end;
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if CurUninstallStep = usUninstall then
  begin
    StopRunningInstance;
    RemoveScheduledTask;
  end
  else if CurUninstallStep = usPostUninstall then
  begin
    if IsSilentUninstall or (MsgBox('是否同时删除配置文件和日志？', mbConfirmation, MB_YESNO) = IDYES) then
      DeleteConfigAndLogs;
  end;
end;
