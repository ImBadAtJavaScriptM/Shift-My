package main

import (
    "log"

    "github.com/ImBadAtJavaScriptM/Shift-My/internal/config"
    "github.com/ImBadAtJavaScriptM/Shift-My/internal/storage"
)

func main() {
    cfg, err := config.Load()
    if err != nil {
        log.Fatal(err)
    }
    store, err := storage.Open(cfg.DBPath)
    if err != nil {
        log.Fatal(err)
    }
    defer store.Close()
}
