Unicode true
RequestExecutionLevel user
Name "AlemonX Desktop"
InstallDir "$LOCALAPPDATA\Programs\AlemonX Desktop"
OutFile "${INSTALLER}"
SetCompressor /SOLID lzma
Icon "alemonx.ico"
UninstallIcon "alemonx.ico"

!include "MUI2.nsh"
!include "x64.nsh"
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "SimpChinese"

Function .onInit
  ${IfNot} ${RunningX64}
    MessageBox MB_ICONSTOP "此安装包需要 64 位 Windows。"
    Abort
  ${EndIf}
FunctionEnd

Section
  SetRegView 32
  ReadRegStr $0 HKLM "Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" "pv"
  ReadRegStr $1 HKCU "Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" "pv"
  ${If} $0 == ""
  ${AndIf} $1 == ""
    InitPluginsDir
    SetOutPath "$PLUGINSDIR"
    File "${BOOTSTRAPPER}"
    ExecWait '"$PLUGINSDIR\MicrosoftEdgeWebview2Setup.exe" /silent /install' $2
    ${If} $2 != 0
      MessageBox MB_ICONSTOP "WebView2 安装失败，请安装 Microsoft Edge WebView2 Runtime 后重试。"
      Abort
    ${EndIf}
  ${EndIf}
  SetOutPath "$INSTDIR"
  File "${PAYLOAD}/AlemonX Desktop.exe"
  File "${PAYLOAD}/alx.exe"
  CreateShortcut "$SMPROGRAMS\AlemonX Desktop.lnk" "$INSTDIR\AlemonX Desktop.exe"
  CreateShortcut "$DESKTOP\AlemonX Desktop.lnk" "$INSTDIR\AlemonX Desktop.exe"
  WriteUninstaller "$INSTDIR\Uninstall.exe"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\AlemonX Desktop" "DisplayName" "AlemonX Desktop"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\AlemonX Desktop" "UninstallString" '"$INSTDIR\Uninstall.exe"'
SectionEnd

Section "Uninstall"
  Delete "$INSTDIR\AlemonX Desktop.exe"
  Delete "$INSTDIR\alx.exe"
  Delete "$INSTDIR\Uninstall.exe"
  Delete "$SMPROGRAMS\AlemonX Desktop.lnk"
  Delete "$DESKTOP\AlemonX Desktop.lnk"
  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\AlemonX Desktop"
  RMDir "$INSTDIR"
SectionEnd
