//go:build lvgl && cgo
#include "bridge.h"
#include "ui_native.h"
#include "catalog.h"
#include <stdlib.h>
#include <string.h>
#include <stdio.h>
#define PAGE_WIDTH 600
static lv_obj_t *catalog_list,*catalog_dots,*catalog_status;
static unsigned catalog_count,catalog_page_index;
typedef struct catalog_entry{char id[64];lv_obj_t *card,*running,*image;lv_image_dsc_t image_data;unsigned char *image_pixels;lv_point_t press;int dragged;struct catalog_entry *next;}catalog_entry;
static catalog_entry *catalog_entries;
static void catalog_page_selected(void){
 unsigned count=lv_obj_get_child_count(catalog_dots);
 int index=(lv_obj_get_scroll_x(catalog_list)+PAGE_WIDTH/2)/PAGE_WIDTH;
 if(index<0)index=0;if(count&&(unsigned)index>=count)index=(int)count-1;
 catalog_page_index=(unsigned)index;
 for(unsigned i=0;i<count;i++)dock_theme_button(lv_obj_get_child(catalog_dots,i),i==catalog_page_index);
}
static void catalog_scroll(lv_event_t *e){(void)e;catalog_page_selected();}
static void catalog_dot_click(lv_event_t *e){unsigned index=(unsigned)(uintptr_t)lv_event_get_user_data(e);lv_obj_scroll_to_x(catalog_list,index*PAGE_WIDTH,LV_ANIM_ON);}
static void catalog_click(lv_event_t *e){
 catalog_entry *entry=lv_event_get_user_data(e);lv_indev_t *input=lv_indev_active();lv_event_code_t code=lv_event_get_code(e);
 if(code==LV_EVENT_PRESSED){entry->dragged=0;if(input)lv_indev_get_point(input,&entry->press);return;}
 if(code==LV_EVENT_PRESSING){if(input){lv_point_t point;lv_indev_get_point(input,&point);if(abs(point.x-entry->press.x)>12||abs(point.y-entry->press.y)>12)entry->dragged=1;}return;}
 if(entry->dragged||(input&&lv_indev_get_scroll_obj(input)))return;
 dock_emit("","shell.open",entry->id);
}
static void catalog_image_release(catalog_entry *entry){
 if(entry->image){lv_obj_delete(entry->image);entry->image=NULL;}
 if(entry->image_pixels){lv_image_cache_drop(&entry->image_data);free(entry->image_pixels);entry->image_pixels=NULL;}
}
static void catalog_deleted(lv_event_t *e){catalog_entry *entry=lv_event_get_user_data(e);catalog_image_release(entry);catalog_entry **cursor=&catalog_entries;while(*cursor&&*cursor!=entry)cursor=&(*cursor)->next;if(*cursor)*cursor=entry->next;free(entry);}
void dock_catalog_create(lv_obj_t *parent){
 catalog_count=catalog_page_index=0;catalog_entries=NULL;
 catalog_list=lv_obj_create(parent);dock_panel(catalog_list,0x0c1220);
 lv_obj_set_pos(catalog_list,20,84);lv_obj_set_size(catalog_list,PAGE_WIDTH,336);
 lv_obj_set_scroll_dir(catalog_list,LV_DIR_HOR);lv_obj_set_scrollbar_mode(catalog_list,LV_SCROLLBAR_MODE_OFF);
 lv_obj_set_scroll_snap_x(catalog_list,LV_SCROLL_SNAP_START);lv_obj_add_flag(catalog_list,LV_OBJ_FLAG_SCROLL_ONE);
 lv_obj_remove_flag(catalog_list,LV_OBJ_FLAG_SCROLL_ELASTIC|LV_OBJ_FLAG_SCROLL_CHAIN);
 lv_obj_set_flex_flow(catalog_list,LV_FLEX_FLOW_ROW);lv_obj_set_style_pad_column(catalog_list,0,0);
 lv_obj_add_event_cb(catalog_list,catalog_scroll,LV_EVENT_SCROLL,NULL);
 catalog_dots=lv_obj_create(parent);dock_panel(catalog_dots,0x0c1220);
 lv_obj_set_pos(catalog_dots,0,432);lv_obj_set_size(catalog_dots,640,14);lv_obj_remove_flag(catalog_dots,LV_OBJ_FLAG_SCROLLABLE);
 lv_obj_set_flex_flow(catalog_dots,LV_FLEX_FLOW_ROW);lv_obj_set_flex_align(catalog_dots,LV_FLEX_ALIGN_CENTER,LV_FLEX_ALIGN_CENTER,LV_FLEX_ALIGN_CENTER);lv_obj_set_style_pad_column(catalog_dots,10,0);
 catalog_status=dock_label(parent,"",26,452,588,dock_font_small,0x94a3b8);lv_obj_set_height(catalog_status,24);lv_label_set_long_mode(catalog_status,LV_LABEL_LONG_DOT);
}
void dock_catalog_add(const char *id,const char *title,const char *description,const char *icon){
 unsigned page_index=catalog_count/4,slot=catalog_count%4;lv_obj_t *page;
 if(slot==0){
  page=lv_obj_create(catalog_list);dock_panel(page,0x0c1220);lv_obj_set_size(page,PAGE_WIDTH,336);lv_obj_remove_flag(page,LV_OBJ_FLAG_SCROLLABLE|LV_OBJ_FLAG_CLICKABLE);lv_obj_add_flag(page,LV_OBJ_FLAG_SNAPPABLE);
  lv_obj_t *dot=lv_obj_create(catalog_dots);dock_panel(dot,0x334155);lv_obj_set_size(dot,10,10);lv_obj_set_style_radius(dot,LV_RADIUS_CIRCLE,0);lv_obj_remove_flag(dot,LV_OBJ_FLAG_SCROLLABLE);
  lv_obj_add_event_cb(dot,catalog_dot_click,LV_EVENT_CLICKED,(void*)(uintptr_t)page_index);dock_theme_button(dot,page_index==catalog_page_index);
 }else page=lv_obj_get_child(catalog_list,page_index);
 catalog_entry *entry=calloc(1,sizeof(*entry));LV_ASSERT_MALLOC(entry);snprintf(entry->id,sizeof(entry->id),"%s",id);
 lv_obj_t *card=entry->card=lv_obj_create(page);dock_panel(card,0x172333);lv_obj_set_pos(card,(slot%2)*308,(slot/2)*172);lv_obj_set_size(card,292,156);lv_obj_set_style_radius(card,18,0);
 dock_theme_background(card,0x203147,LV_STATE_PRESSED);lv_obj_remove_flag(card,LV_OBJ_FLAG_SCROLLABLE);lv_obj_add_flag(card,LV_OBJ_FLAG_CLICKABLE);
 lv_obj_add_event_cb(card,catalog_click,LV_EVENT_CLICKED,entry);lv_obj_add_event_cb(card,catalog_click,LV_EVENT_PRESSED,entry);lv_obj_add_event_cb(card,catalog_click,LV_EVENT_PRESSING,entry);lv_obj_add_event_cb(card,catalog_deleted,LV_EVENT_DELETE,entry);
 lv_obj_t *badge=lv_obj_create(card);dock_panel(badge,0x134e4a);lv_obj_set_pos(badge,18,16);lv_obj_set_size(badge,52,52);lv_obj_set_style_radius(badge,14,0);lv_obj_remove_flag(badge,LV_OBJ_FLAG_CLICKABLE|LV_OBJ_FLAG_SCROLLABLE);
 lv_obj_t *symbol=dock_label(badge,icon,0,0,52,dock_font_title,0x5eead4);lv_obj_set_style_text_align(symbol,LV_TEXT_ALIGN_CENTER,0);
 if((unsigned char)icon[0]>=0x80){lv_obj_set_style_transform_pivot_x(symbol,lv_pct(50),0);lv_obj_set_style_transform_pivot_y(symbol,lv_pct(50),0);lv_obj_set_style_transform_scale(symbol,384,0);}
 lv_obj_center(symbol);
 entry->running=dock_label(card,"运行中",194,16,84,dock_font_small,0x5eead4);lv_obj_set_style_text_align(entry->running,LV_TEXT_ALIGN_RIGHT,0);lv_obj_add_flag(entry->running,LV_OBJ_FLAG_HIDDEN);
 lv_obj_t *label=dock_label(card,title,18,82,256,dock_font_body,0xf8fafc);lv_obj_set_height(label,28);lv_label_set_long_mode(label,LV_LABEL_LONG_DOT);
 label=dock_label(card,description,18,118,256,dock_font_small,0x94a3b8);lv_obj_set_height(label,24);lv_label_set_long_mode(label,LV_LABEL_LONG_DOT);
 entry->next=catalog_entries;catalog_entries=entry;catalog_count++;
}
void dock_catalog_running(const char *id,int running){for(catalog_entry *entry=catalog_entries;entry;entry=entry->next){if(!strcmp(entry->id,id)){if(running)lv_obj_remove_flag(entry->running,LV_OBJ_FLAG_HIDDEN);else lv_obj_add_flag(entry->running,LV_OBJ_FLAG_HIDDEN);return;}}}
void dock_catalog_status(const char *status){lv_label_set_text(catalog_status,status?status:"");}

int dock_catalog_image(const char *id,const unsigned char *pixels,int width,int height){
 if(!pixels||width<1||height<1||width>512||height>512)return -1;
 catalog_entry *entry=catalog_entries;while(entry&&strcmp(entry->id,id))entry=entry->next;if(!entry)return -2;
 size_t size=(size_t)width*height*4;unsigned char *copy=malloc(size);if(!copy)return -3;memcpy(copy,pixels,size);
 catalog_image_release(entry);lv_obj_t *badge=lv_obj_get_child(entry->card,0);lv_obj_clean(badge);
 entry->image_pixels=copy;entry->image_data=(lv_image_dsc_t){.header={.magic=LV_IMAGE_HEADER_MAGIC,.cf=LV_COLOR_FORMAT_ARGB8888,.w=width,.h=height,.stride=width*4},.data_size=size,.data=copy};
 entry->image=lv_image_create(badge);lv_image_set_src(entry->image,&entry->image_data);
 lv_obj_set_size(entry->image,48,48);lv_image_set_inner_align(entry->image,LV_IMAGE_ALIGN_CONTAIN);lv_image_set_antialias(entry->image,true);lv_obj_center(entry->image);
 lv_obj_set_style_image_recolor_opa(entry->image,LV_OPA_TRANSP,0);lv_obj_remove_flag(entry->image,LV_OBJ_FLAG_CLICKABLE|LV_OBJ_FLAG_SCROLLABLE);
 return 0;
}
