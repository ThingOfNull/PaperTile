@echo off
rem ============================================================
rem  build.bat  one-click build for go-printtile-pro
rem  Pipeline: toolchain check -> frontend deps -> go vet -> go test -> wails build
rem  Output  : .\build\bin\go-printtile-pro.exe
rem
rem  NOTE: Keep this file ASCII-only. Chinese Windows cmd.exe parses
rem        batch files with the active OEM codepage (GBK), so any
rem        non-ASCII character saved as UTF-8 will break the parser.
rem ============================================================
setlocal
cd /d "%~dp0"

if not exist "internal\secrets\secrets.json" (
    echo [INFO] internal\secrets\secrets.json missing - copying secrets.example.json
    copy /Y "internal\secrets\secrets.example.json" "internal\secrets\secrets.json" >nul
    if errorlevel 1 (
        echo [ERR] could not create internal\secrets\secrets.json
        goto :fail
    )
)

echo.
echo [1/5] Checking toolchain...
where go >nul 2>nul
if errorlevel 1 (
    echo [ERR] go not found. Please install Go and add go.exe to PATH.
    goto :fail
)
where npm >nul 2>nul
if errorlevel 1 (
    echo [ERR] npm not found. Please install Node.js.
    goto :fail
)
where wails >nul 2>nul
if errorlevel 1 (
    echo [INFO] wails CLI not found, installing...
    go install github.com/wailsapp/wails/v2/cmd/wails@latest
    if errorlevel 1 (
        echo [ERR] wails install failed. Run manually:
        echo       go install github.com/wailsapp/wails/v2/cmd/wails@latest
        goto :fail
    )
)

echo.
echo [2/5] Installing frontend dependencies...
pushd frontend
if not exist node_modules (
    call npm install
    if errorlevel 1 (
        popd
        echo [ERR] npm install failed.
        goto :fail
    )
) else (
    echo      node_modules already exists, skipped.
    echo      Delete frontend\node_modules to force a fresh install.
)
popd

echo.
echo [3/5] go vet...
go vet ./...
if errorlevel 1 (
    echo [ERR] go vet failed.
    goto :fail
)

echo.
echo [4/5] go test...
go test ./...
if errorlevel 1 (
    echo [ERR] unit tests failed.
    goto :fail
)

echo.
echo [5/5] wails build...
rem Produce a fully static, single-file Windows exe:
rem   CGO_ENABLED=0        - no MinGW/MSVC runtime dependency (Wails v2 does not need cgo on Windows).
rem   -clean               - wipe the old build\bin before linking.
rem   -trimpath            - strip absolute source paths from the binary.
rem   -ldflags "-s -w"     - drop debug info and symbol table, ~30%% smaller exe.
rem   -webview2 embed      - embed WebView2Loader.dll into the exe resource; no loose DLL beside exe.
rem
rem NOTE: the Microsoft WebView2 Runtime itself is an OS-level component (bundled with Edge
rem on Windows 10 1809+/11), so the exe still relies on it being present at runtime. If you
rem need to ship to machines that have no Runtime, use `-webview2 browser` + bundle the
rem Evergreen Bootstrapper, or switch to a fixed-version runtime install.
set CGO_ENABLED=0
wails build -clean -trimpath -ldflags "-s -w" -webview2 embed
if errorlevel 1 (
    echo [ERR] wails build failed.
    goto :fail
)

echo.
echo ============================================================
echo  Build succeeded: %cd%\build\bin\go-printtile-pro.exe
echo ============================================================
endlocal
pause
exit /b 0

:fail
echo.
echo Build FAILED, see log above.
endlocal
pause
exit /b 1
