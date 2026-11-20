//go:build lvgl && cgo
#include "bridge.h"
#include "ui_native.h"
#include <string.h>
#include <stdlib.h>
#include <stdio.h>
static lv_obj_t *screensaver,*saver_clock,*saver_date,*saver_pet_image,*saver_pet_status,*saver_pet_datetime,*saver_pet_bubble,*saver_pet_title,*saver_pet_text;
static lv_obj_t *saver_previous;
static uint8_t *saver_pet_pixels;
static lv_image_dsc_t saver_pet_frame;
static void saver_screen_clicked(lv_event_t *e){
 (void)e;
 if(!lv_obj_has_flag(saver_pet_image,LV_OBJ_FLAG_HIDDEN)){
  lv_point_t point={320,240};lv_indev_t *input=lv_indev_active();if(input)lv_indev_get_point(input,&point);
  char x[16];snprintf(x,sizeof(x),"%d",(int)point.x);dock_emit("","shell.pet.touch",x);
 }else dock_emit("","shell.wake",NULL);
}
void dock_screensaver_show(const char *kind){
 if(!screensaver){
  saver_previous=lv_screen_active();
  screensaver=lv_obj_create(NULL);dock_panel(screensaver,0x000000);
  lv_obj_remove_flag(screensaver,LV_OBJ_FLAG_SCROLLABLE);
  lv_obj_add_event_cb(screensaver,saver_screen_clicked,LV_EVENT_CLICKED,NULL);
  saver_pet_image=lv_image_create(screensaver);lv_obj_set_pos(saver_pet_image,0,0);
  lv_obj_remove_flag(saver_pet_image,LV_OBJ_FLAG_CLICKABLE);
  saver_clock=dock_label(screensaver,"等待对时",0,176,640,dock_font_clock,0xffffff);
  lv_obj_set_style_text_align(saver_clock,LV_TEXT_ALIGN_CENTER,0);
  saver_date=dock_label(screensaver,"",0,272,640,dock_font_body,0x94a3b8);lv_obj_set_style_text_align(saver_date,LV_TEXT_ALIGN_CENTER,0);
  lv_obj_remove_flag(saver_clock,LV_OBJ_FLAG_CLICKABLE);lv_obj_remove_flag(saver_date,LV_OBJ_FLAG_CLICKABLE);
  saver_pet_status=dock_label(screensaver,"",16,12,392,dock_font_body,0xffffff);
  lv_obj_set_style_bg_color(saver_pet_status,dock_color(0x000000),0);lv_obj_set_style_bg_opa(saver_pet_status,LV_OPA_70,0);
  lv_label_set_long_mode(saver_pet_status,LV_LABEL_LONG_DOT);lv_obj_set_height(saver_pet_status,32);
  saver_pet_datetime=dock_label(screensaver,"",420,12,204,dock_font_body,0xffffff);
  lv_obj_set_style_text_align(saver_pet_datetime,LV_TEXT_ALIGN_RIGHT,0);
  lv_obj_set_style_bg_color(saver_pet_datetime,dock_color(0x000000),0);lv_obj_set_style_bg_opa(saver_pet_datetime,LV_OPA_70,0);
  lv_obj_remove_flag(saver_pet_datetime,LV_OBJ_FLAG_CLICKABLE);
  saver_pet_bubble=lv_obj_create(screensaver);lv_obj_set_pos(saver_pet_bubble,12,358);lv_obj_set_size(saver_pet_bubble,616,110);
  lv_obj_set_style_bg_color(saver_pet_bubble,dock_color(0x000000),0);lv_obj_set_style_bg_opa(saver_pet_bubble,LV_OPA_80,0);
  lv_obj_set_style_border_width(saver_pet_bubble,0,0);lv_obj_set_style_pad_all(saver_pet_bubble,8,0);lv_obj_set_style_radius(saver_pet_bubble,12,0);
  lv_obj_remove_flag(saver_pet_bubble,LV_OBJ_FLAG_CLICKABLE|LV_OBJ_FLAG_SCROLLABLE);
  saver_pet_title=dock_label(saver_pet_bubble,"",0,0,596,dock_font_body,0xffffff);lv_label_set_long_mode(saver_pet_title,LV_LABEL_LONG_DOT);lv_obj_set_height(saver_pet_title,32);
  saver_pet_text=dock_label(saver_pet_bubble,"",0,36,596,dock_font_body,0xffffff);lv_label_set_long_mode(saver_pet_text,LV_LABEL_LONG_DOT);lv_obj_set_height(saver_pet_text,56);
  lv_obj_remove_flag(saver_pet_status,LV_OBJ_FLAG_CLICKABLE);lv_obj_remove_flag(saver_pet_title,LV_OBJ_FLAG_CLICKABLE);lv_obj_remove_flag(saver_pet_text,LV_OBJ_FLAG_CLICKABLE);
 }
 int pet=!strcmp(kind,"pet");
 lv_obj_t *pet_parts[]={saver_pet_image,saver_pet_status,saver_pet_datetime,saver_pet_bubble};
 for(unsigned i=0;i<sizeof(pet_parts)/sizeof(pet_parts[0]);i++){if(pet)lv_obj_remove_flag(pet_parts[i],LV_OBJ_FLAG_HIDDEN);else lv_obj_add_flag(pet_parts[i],LV_OBJ_FLAG_HIDDEN);}
 if(!strcmp(kind,"blank")||pet){lv_obj_add_flag(saver_clock,LV_OBJ_FLAG_HIDDEN);lv_obj_add_flag(saver_date,LV_OBJ_FLAG_HIDDEN);}
 else{lv_obj_remove_flag(saver_clock,LV_OBJ_FLAG_HIDDEN);lv_obj_remove_flag(saver_date,LV_OBJ_FLAG_HIDDEN);}
 /* A short native fade softens the high-contrast framebuffer change. Keep
    both screens owned by the shell; an immediate wake must never auto-delete
    the catalog. Start now so wake can cancel even before the next UI tick. */
 lv_screen_load_anim(screensaver,LV_SCR_LOAD_ANIM_FADE_IN,180,0,false);
 lv_anim_refr_now();
}
void dock_screensaver_time(const char *clock,const char *date){
 if(!screensaver)return;
 lv_label_set_text(saver_clock,clock);lv_label_set_text(saver_date,date);
 if(*date)lv_label_set_text_fmt(saver_pet_datetime,"%.5s",clock);else lv_label_set_text(saver_pet_datetime,clock);
}
int dock_screensaver_frame(const unsigned char *pixels,unsigned size){
 if(!screensaver||!pixels||size!=640*480*4||lv_obj_has_flag(saver_pet_image,LV_OBJ_FLAG_HIDDEN))return -1;
 if(!saver_pet_pixels){
  saver_pet_pixels=malloc(size);if(!saver_pet_pixels)return -2;
  memset(&saver_pet_frame,0,sizeof(saver_pet_frame));saver_pet_frame.header.magic=LV_IMAGE_HEADER_MAGIC;
  saver_pet_frame.header.cf=LV_COLOR_FORMAT_XRGB8888;saver_pet_frame.header.w=640;saver_pet_frame.header.h=480;saver_pet_frame.header.stride=640*4;
  saver_pet_frame.data_size=size;saver_pet_frame.data=saver_pet_pixels;
 }
 memcpy(saver_pet_pixels,pixels,size);lv_image_set_src(saver_pet_image,&saver_pet_frame);lv_obj_invalidate(saver_pet_image);return 0;
}
void dock_screensaver_pet(const char *status,const char *title,const char *text){
 if(!screensaver)return;
 lv_label_set_text(saver_pet_status,status);lv_label_set_text(saver_pet_title,title);lv_label_set_text(saver_pet_text,text);
 if(!*title&&!*text)lv_obj_add_flag(saver_pet_bubble,LV_OBJ_FLAG_HIDDEN);else lv_obj_remove_flag(saver_pet_bubble,LV_OBJ_FLAG_HIDDEN);
}
void dock_screensaver_close(void){
 if(screensaver){lv_obj_delete(screensaver);screensaver=saver_clock=saver_date=saver_pet_image=saver_pet_status=saver_pet_datetime=saver_pet_bubble=saver_pet_title=saver_pet_text=NULL;}
 free(saver_pet_pixels);saver_pet_pixels=NULL;memset(&saver_pet_frame,0,sizeof(saver_pet_frame));
 saver_previous=NULL;
}
void dock_screensaver_restore(void){if(saver_previous)lv_screen_load(saver_previous);dock_screensaver_close();}
