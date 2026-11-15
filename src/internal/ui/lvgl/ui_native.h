#ifndef HACHIMODOCK_UI_NATIVE_H
#define HACHIMODOCK_UI_NATIVE_H
#include "lvgl.h"
extern lv_font_t *dock_font_body,*dock_font_title,*dock_font_small,*dock_font_clock;
lv_color_t dock_color(unsigned rgb);
lv_obj_t *dock_textarea_create(lv_obj_t *parent);
lv_obj_t *dock_keyboard_create(lv_obj_t *parent);
lv_obj_t *dock_switch_create(lv_obj_t *parent);
lv_obj_t *dock_label(lv_obj_t *parent,const char *text,int x,int y,int width,const lv_font_t *font,unsigned fg);
void dock_panel(lv_obj_t *obj,unsigned bg);
lv_obj_t *dock_button(lv_obj_t *parent,const char *text,int x,int y,int width,int height,unsigned bg,const char *app_id,const char *action);
void dock_theme_init(lv_display_t *display);
void dock_theme_close(void);
void dock_theme_button(lv_obj_t *obj,int active);
void dock_theme_focus(lv_obj_t *obj);
void dock_theme_surface(lv_obj_t *obj);
void dock_theme_text(lv_obj_t *obj);
void dock_theme_background(lv_obj_t *obj,unsigned rgb,lv_style_selector_t selector);
void dock_theme_foreground(lv_obj_t *obj,unsigned rgb,lv_style_selector_t selector);
unsigned dock_theme_resolve(unsigned rgb);
#endif
