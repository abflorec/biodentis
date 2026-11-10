//go:build windows && desktop

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
	"unsafe"

	webview2 "github.com/jchv/go-webview2"
	"github.com/jchv/go-webview2/webviewloader"
	"golang.org/x/sys/windows"
)

func main() {
	data := flag.String("data", "", "Carpeta independiente para la demo de escritorio")
	flag.Parse()
	if err := runDesktop(*data); err != nil {
		message, _ := windows.UTF16PtrFromString(err.Error())
		title, _ := windows.UTF16PtrFromString("BIODENTIS Escritorio")
		messageBox := windows.NewLazySystemDLL("user32.dll").NewProc("MessageBoxW")
		messageBox.Call(0, uintptr(unsafe.Pointer(message)), uintptr(unsafe.Pointer(title)), 0x10)
		os.Exit(1)
	}
}

func runDesktop(root string) error {
	if _, err := webviewloader.GetInstalledVersion(); err != nil {
		return errors.New("Instala Microsoft Edge WebView2 Runtime y vuelve a abrir BIODENTIS Escritorio.")
	}
	if root == "" {
		local, err := os.UserCacheDir()
		if err != nil {
			return fmt.Errorf("abrir carpeta de datos de Windows: %w", err)
		}
		root = filepath.Join(local, "BIODENTIS", "EscritorioDemo")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return fmt.Errorf("crear carpeta de datos: %w", err)
	}
	lock, err := lockDesktopProfile(root)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(lock)

	logFile, err := os.OpenFile(filepath.Join(root, "arranque.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("abrir registro de arranque: %w", err)
	}
	defer logFile.Close()
	log.SetOutput(logFile)

	dataDir := filepath.Join(root, "datos")
	db, err := openDatabase(dataDir, true)
	if err != nil {
		return fmt.Errorf("abrir base de escritorio: %w", err)
	}
	defer db.Close()

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("iniciar servicio local: %w", err)
	}
	defer listener.Close()
	address := listener.Addr().String()
	app := &server{db: db, dataDir: dataDir, demo: true, sessions: make(map[string]loginSession)}
	routes := app.routes()
	httpServer := localHTTPServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != address {
			http.Error(w, "Dirección local incorrecta.", http.StatusForbidden)
			return
		}
		routes.ServeHTTP(w, r)
	}))
	go func() {
		if err := httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("Servicio local: %v", err)
		}
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(ctx); err != nil {
			_ = httpServer.Close()
		}
	}()

	view := webview2.NewWithOptions(webview2.WebViewOptions{
		DataPath:  filepath.Join(root, "webview"),
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title: "BIODENTIS · Escritorio", Width: 1200, Height: 760, Center: true,
		},
	})
	if view == nil {
		return errors.New("No se pudo abrir la ventana de BIODENTIS. Consulta arranque.log en la carpeta de datos.")
	}
	defer view.Destroy()
	view.SetSize(680, 490, webview2.HintMin)
	view.Navigate("http://" + address)
	view.Run()
	return nil
}

func lockDesktopProfile(root string) (windows.Handle, error) {
	name, err := windows.UTF16PtrFromString(filepath.Join(root, "desktop.lock"))
	if err != nil {
		return windows.InvalidHandle, err
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
		return windows.InvalidHandle, errors.New("BIODENTIS Escritorio ya está abierto para esta carpeta de datos.")
	}
	if err != nil {
		return windows.InvalidHandle, fmt.Errorf("bloquear carpeta de datos de escritorio: %w", err)
	}
	return handle, nil
}
