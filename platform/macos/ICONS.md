# DogClaw 图标说明

## ✅ 状态栏图标（已完成）

状态栏图标目前使用 emoji "🐾"（狗爪印），这个是在代码中直接设置的，无需额外配置。

## 应用图标（可选）

应用图标需要创建一个 `.icns` 文件。以下是添加自定义图标的几种方法：

### 方法 1：使用在线工具（最简单）

1. 访问 [cloudconvert.com/png-to-icns](https://cloudconvert.com/png-to-icns)
2. 上传一张 1024x1024 像素的 PNG 图片
3. 转换并下载 `.icns` 文件
4. 将文件重命名为 `AppIcon.icns`
5. 放到 `platform/macos/` 目录下
6. 重新运行 `build.sh`

### 方法 2：使用 emoji 作为图标

你可以下载现成的 emoji 图标：
- 访问 [emojipedia.org/paw-prints](https://emojipedia.org/paw-prints/)
- 右键保存图片
- 使用方法 1 转换为 ICNS

### 方法 3：使用 macOS 自带工具

1. 准备一张 1024x1024 像素的 PNG 图片
2. 创建一个名为 `AppIcon.iconset` 的文件夹
3. 在该文件夹中放置以下尺寸的图片文件：
   - icon_16x16.png (16x16)
   - icon_16x16@2x.png (32x32)
   - icon_32x32.png (32x32)
   - icon_32x32@2x.png (64x64)
   - icon_128x128.png (128x128)
   - icon_128x128@2x.png (256x256)
   - icon_256x256.png (256x256)
   - icon_256x256@2x.png (512x512)
   - icon_512x512.png (512x512)
   - icon_512x512@2x.png (1024x1024)

4. 在终端运行：
   ```bash
   iconutil -c icns AppIcon.iconset
   ```

5. 将生成的 `AppIcon.icns` 放到 `platform/macos/` 目录下

## 当前状态

- ✅ 状态栏图标：已配置（🐾 狗爪印）
- ⏳ 应用图标：等待添加 `AppIcon.icns` 文件

## 提示

即使没有应用图标，应用也能正常运行！状态栏会显示 🐾 图标。
