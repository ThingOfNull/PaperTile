# PaperTile / 自动切图

桌面打印切图与拼版工具，支持自动分页导出 PDF。  
Desktop image tiling and print-imposition tool with automatic PDF pagination.

## 中文

### 项目简介

`PaperTile` 是一个基于 `Wails + Go + Vue` 的桌面应用，用于将大图切分为多页并导出打印 PDF，适合海报拼接、工程图分幅打印等场景。

### 功能

- 按纸张尺寸自动切图分页
- 支持边距、重叠、方向、目标尺寸设置
- 支持标准模式与省纸模式
- 支持导出 PDF
- 可选接入百度云图像增强

### 环境要求

- Go 1.21+
- Node.js 18+
- Wails CLI

### 开发运行

```bash
wails dev
```

### 构建发布

```bash
wails build
```

### 百度云配置

应用运行后可在界面中点击“配置百度云”填写 AK/SK。  
配置会保存到二进制同级目录下：

- `config/config.json`

### 许可证

MIT，详见 `LICENSE`。

---

## English

### Overview

`PaperTile` is a desktop app built with `Wails + Go + Vue` for splitting large images into printable pages and exporting PDF files, suitable for poster assembly and large-format print workflows.

### Features

- Automatic image tiling by paper size
- Adjustable margin, overlap, orientation, and target size
- Standard mode and paper-saving mode
- PDF export
- Optional Baidu Cloud image enhancement

### Requirements

- Go 1.21+
- Node.js 18+
- Wails CLI

### Development

```bash
wails dev
```

### Build

```bash
wails build
```

### Baidu Cloud Config

Use the in-app "Configure Baidu Cloud" dialog to set AK/SK.  
The config is stored next to the executable at:

- `config/config.json`

### License

MIT. See `LICENSE` for details.
