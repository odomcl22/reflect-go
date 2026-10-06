// Small 32-bit entry point works on Windows x64 and ARM64 and selects native code.
#include <windows.h>
#include <wchar.h>

int WINAPI wWinMain(HINSTANCE instance, HINSTANCE previous, PWSTR command, int show) {
    (void)instance; (void)previous; (void)command; (void)show;
    wchar_t path[32768];
    DWORD length=GetModuleFileNameW(NULL,path,32768);
    if (!length || length>=32768) return 1;
    wchar_t *last=wcsrchr(path,L'\\'); if (!last) return 1; *last=0;
    SYSTEM_INFO info; GetNativeSystemInfo(&info);
    // Native-machine reporting avoids guessing from the architecture of this
    // small entry point when Windows is emulating it on ARM64.
    typedef BOOL (WINAPI *MachineQuery)(HANDLE, USHORT *, USHORT *);
    MachineQuery query=(MachineQuery)(void *)GetProcAddress(GetModuleHandleW(L"kernel32.dll"),"IsWow64Process2");
    USHORT processMachine=0,nativeMachine=0;
    if (query && query(GetCurrentProcess(),&processMachine,&nativeMachine)) {
        if (nativeMachine==IMAGE_FILE_MACHINE_ARM64) info.wProcessorArchitecture=PROCESSOR_ARCHITECTURE_ARM64;
        if (nativeMachine==IMAGE_FILE_MACHINE_AMD64) info.wProcessorArchitecture=PROCESSOR_ARCHITECTURE_AMD64;
    }
    const wchar_t *relative=NULL;
    if (info.wProcessorArchitecture==PROCESSOR_ARCHITECTURE_AMD64) relative=L"\\bin\\windows-x64\\reflectgo-window.exe";
    if (info.wProcessorArchitecture==PROCESSOR_ARCHITECTURE_ARM64) relative=L"\\bin\\windows-arm64\\reflectgo-window.exe";
    if (!relative) { MessageBoxW(NULL,L"Reflect Go needs 64-bit Windows on an Intel/AMD or ARM computer.",L"Reflect Go",MB_OK|MB_ICONERROR); return 1; }
    if (wcslen(path)+wcslen(relative)+1>=32768) return 1;
    wcscat(path,relative);
    SHELLEXECUTEINFOW execute={0};execute.cbSize=sizeof(execute);execute.fMask=SEE_MASK_NOCLOSEPROCESS|SEE_MASK_FLAG_NO_UI;
    execute.lpVerb=L"open";execute.lpFile=path;execute.lpParameters=L"ui";execute.nShow=SW_SHOWNORMAL;
    if (!ShellExecuteExW(&execute)) { MessageBoxW(NULL,L"Reflect Go could not start. Keep START Reflect Go.exe inside the complete Phase 4.5 folder. If Windows showed a security or device-policy message, save that message for diagnosis.",L"Reflect Go",MB_OK|MB_ICONERROR); return 1; }
    if (execute.hProcess) CloseHandle(execute.hProcess);
    return 0;
}
