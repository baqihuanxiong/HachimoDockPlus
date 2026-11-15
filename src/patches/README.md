# LVGL 本地修复

`lvgl-span-line-cache.patch` 对应固定的 LVGL v9.3.0 提交
`c033a98afddd65aaafeebea625382a94020fe4a7`。`bootstrap.sh` 和 `build.sh`
通过 `prepare-lvgl.sh` 幂等应用补丁。

原始富文本组件每次绘制都从头计算行位置。补丁在首次绘制时记录各行的
span、UTF-8 字节偏移及纵向坐标，后续通过二分查找跳过视口上方的文本。
文本、字体、样式或宽度变化时重建索引；销毁对象时释放索引。
索引来自实际绘制过程，保留原有跨 span 的换行、字体基线和 Markdown 样式。

`tests/picoclaw_scroll.c` 比较缓存与原始路径的完整像素，覆盖连续长段落、
多行富文本、宽度及主题变化，并检查滚动工作量不随文本总长度线性增长。
升级 LVGL 时需要重新核对补丁和这些测试。
