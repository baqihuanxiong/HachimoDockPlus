#ifndef HACHIMODOCK_UI_NATIVE_H
#define HACHIMODOCK_UI_NATIVE_H
#include "lvgl.h"
extern lv_font_t *dock_font_body,*dock_font_title,*dock_font_small;
lv_color_t dock_color(unsigned rgb);
lv_obj_t *dock_label(lv_obj_t *parent,const char *text,int x,int y,int width,const lv_font_t *font,unsigned fg);
void dock_panel(lv_obj_t *obj,unsigned bg);
lv_obj_t *dock_button(lv_obj_t *parent,const char *text,int x,int y,int width,int height,unsigned bg,const char *app_id,const char *action);
#endif
