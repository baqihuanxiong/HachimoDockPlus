//go:build lvgl && cgo
#include "bridge.h"
#include "ui_native.h"
#include <string.h>
static lv_obj_t *screensaver,*saver_clock,*saver_date;
static void saver_wake(lv_event_t *e){(void)e;dock_emit("","shell.wake",NULL);}
void dock_screensaver_show(const char *kind){
 if(!screensaver){
  screensaver=lv_obj_create(NULL);dock_panel(screensaver,0x000000);
  lv_obj_remove_flag(screensaver,LV_OBJ_FLAG_SCROLLABLE);
  lv_obj_add_event_cb(screensaver,saver_wake,LV_EVENT_CLICKED,NULL);
  saver_clock=dock_label(screensaver,"等待对时",0,176,640,dock_font_clock,0xffffff);
  lv_obj_set_style_text_align(saver_clock,LV_TEXT_ALIGN_CENTER,0);
  saver_date=dock_label(screensaver,"",0,272,640,dock_font_body,0x94a3b8);lv_obj_set_style_text_align(saver_date,LV_TEXT_ALIGN_CENTER,0);
  lv_obj_remove_flag(saver_clock,LV_OBJ_FLAG_CLICKABLE);lv_obj_remove_flag(saver_date,LV_OBJ_FLAG_CLICKABLE);
 }
 if(!strcmp(kind,"blank")){lv_obj_add_flag(saver_clock,LV_OBJ_FLAG_HIDDEN);lv_obj_add_flag(saver_date,LV_OBJ_FLAG_HIDDEN);}
 else{lv_obj_remove_flag(saver_clock,LV_OBJ_FLAG_HIDDEN);lv_obj_remove_flag(saver_date,LV_OBJ_FLAG_HIDDEN);}
 lv_screen_load(screensaver);
}
void dock_screensaver_time(const char *clock,const char *date){
 if(!screensaver)return;
 lv_label_set_text(saver_clock,clock);lv_label_set_text(saver_date,date);
}
void dock_screensaver_close(void){if(screensaver){lv_obj_delete(screensaver);screensaver=saver_clock=saver_date=NULL;}}
