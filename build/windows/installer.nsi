; The WhatsApp MCP installer for Windows: per user, without asking for an
; administrator, always into the same folder, because Claude Desktop's
; configuration and "open at login" point at the programs there.
;
;   makensis -DVERSION=1.2.3 -DSRC=<folder with the programs> -DOUT=<setup.exe> installer.nsi
;
; /S installs silently. /UPDATE=<pid> is how the app updates itself: wait for
; that process to end, install, and open the app again.

Unicode true
ManifestDPIAware true
RequestExecutionLevel user
SetCompressor /SOLID lzma

!include "MUI2.nsh"
!include "FileFunc.nsh"
!include "LogicLib.nsh"

!define PRODUCT "WhatsApp MCP"
!define EXE "WhatsApp MCP.exe"
!define UNINST_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\WhatsAppMCP"
!define RUN_KEY "Software\Microsoft\Windows\CurrentVersion\Run"
!define WEBVIEW2_GUID "{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}"

Name "${PRODUCT}"
OutFile "${OUT}"
InstallDir "$LOCALAPPDATA\Programs\${PRODUCT}"
BrandingText "${PRODUCT} ${VERSION}"

VIProductVersion "${VERSION4}"
VIFileVersion "${VERSION4}"
VIAddVersionKey "ProductName" "${PRODUCT}"
VIAddVersionKey "CompanyName" "Bruno Orlandi"
VIAddVersionKey "FileDescription" "${PRODUCT} — instalador"
VIAddVersionKey "FileVersion" "${VERSION}"
VIAddVersionKey "ProductVersion" "${VERSION}"
VIAddVersionKey "LegalCopyright" "© Bruno Orlandi. MIT License."

!define MUI_ICON "icon.ico"
!define MUI_UNICON "icon.ico"
!define MUI_ABORTWARNING
!define MUI_FINISHPAGE_RUN "$INSTDIR\${EXE}"
!define MUI_FINISHPAGE_RUN_TEXT "Abrir o ${PRODUCT}"
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "PortugueseBR"

Var UpdatePID

Function .onInit
  ${GetParameters} $R0
  ClearErrors
  ${GetOptions} $R0 "/UPDATE=" $UpdatePID
  ${If} ${Errors}
    StrCpy $UpdatePID ""
  ${EndIf}
FunctionEnd

; waitFor waits up to a minute for the app that asked for the update to quit,
; which it does on its own after starting this installer.
Function waitFor
  ${If} $UpdatePID != ""
    System::Call 'kernel32::OpenProcess(i 0x00100000, i 0, i $UpdatePID) p .r1'
    ${If} $1 P<> 0
      System::Call 'kernel32::WaitForSingleObject(p r1, i 60000) i .r2'
      System::Call 'kernel32::CloseHandle(p r1)'
    ${EndIf}
  ${EndIf}
FunctionEnd

Section "Install"
  Call waitFor
  ; An app left open would keep its programs locked.
  nsExec::Exec 'taskkill /F /IM "${EXE}"'
  Sleep 800

  ; The window draws with WebView2, which Windows 11 and an up-to-date
  ; Windows 10 already have.
  ReadRegStr $0 HKLM "SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\${WEBVIEW2_GUID}" "pv"
  ReadRegStr $1 HKCU "Software\Microsoft\EdgeUpdate\Clients\${WEBVIEW2_GUID}" "pv"
  ${If} $0 == ""
  ${AndIf} $1 == ""
    DetailPrint "Instalando o WebView2…"
    SetOutPath "$PLUGINSDIR"
    File "MicrosoftEdgeWebview2Setup.exe"
    ExecWait '"$PLUGINSDIR\MicrosoftEdgeWebview2Setup.exe" /silent /install'
  ${EndIf}

  SetOutPath "$INSTDIR"
  File "${SRC}\${EXE}"
  File "${SRC}\whatsapp-mcp-bridge.exe"
  File "${SRC}\THIRD-PARTY-NOTICES.txt"
  SetOutPath "$INSTDIR\bin"
  File "${SRC}\bin\wacli.exe"
  SetOutPath "$INSTDIR"

  CreateShortcut "$SMPROGRAMS\${PRODUCT}.lnk" "$INSTDIR\${EXE}"

  WriteUninstaller "$INSTDIR\uninstall.exe"
  WriteRegStr HKCU "${UNINST_KEY}" "DisplayName" "${PRODUCT}"
  WriteRegStr HKCU "${UNINST_KEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "${UNINST_KEY}" "Publisher" "Bruno Orlandi"
  WriteRegStr HKCU "${UNINST_KEY}" "DisplayIcon" "$INSTDIR\${EXE}"
  WriteRegStr HKCU "${UNINST_KEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${UNINST_KEY}" "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteRegStr HKCU "${UNINST_KEY}" "QuietUninstallString" '"$INSTDIR\uninstall.exe" /S'
  WriteRegStr HKCU "${UNINST_KEY}" "URLInfoAbout" "https://github.com/BrOrlandi/whatsapp-mcp-v2"
  WriteRegDWORD HKCU "${UNINST_KEY}" "NoModify" 1
  WriteRegDWORD HKCU "${UNINST_KEY}" "NoRepair" 1
  ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
  WriteRegDWORD HKCU "${UNINST_KEY}" "EstimatedSize" $0

  ${If} $UpdatePID != ""
    Exec '"$INSTDIR\${EXE}"'
  ${EndIf}
SectionEnd

; Uninstalling removes the programs, not what the app keeps in
; %LOCALAPPDATA%\WhatsApp MCP: the messages, the WhatsApp session and the
; transcription model stay until the app's "Apagar todos os dados" or a
; manual delete.
Section "Uninstall"
  nsExec::Exec 'taskkill /F /IM "${EXE}"'
  Sleep 800
  Delete "$INSTDIR\${EXE}"
  Delete "$INSTDIR\whatsapp-mcp-bridge.exe"
  Delete "$INSTDIR\THIRD-PARTY-NOTICES.txt"
  Delete "$INSTDIR\bin\wacli.exe"
  RMDir "$INSTDIR\bin"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"
  Delete "$SMPROGRAMS\${PRODUCT}.lnk"
  DeleteRegValue HKCU "${RUN_KEY}" "com.brorlandi.whatsapp-mcp"
  RMDir /r "$APPDATA\${EXE}"
  DeleteRegKey HKCU "${UNINST_KEY}"
SectionEnd
