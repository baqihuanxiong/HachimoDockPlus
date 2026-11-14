//go:build lvgl && cgo
#include "bridge.h"
#include "ui_native.h"
#include "statusbar.h"
#include <string.h>

static lv_obj_t *header_clock,*header_battery,*header_wifi,*header_bluetooth;
static int header_battery_level=-1,header_charging,header_wifi_level,header_bluetooth_kind;

static unsigned battery_color(int level){return level<0?0x64748b:level<10?0xf87171:level<20?0xfb923c:0x4ade80;}
static void status_rect(lv_layer_t *layer,int x,int y,int w,int h,unsigned color,int border,int radius){
 lv_draw_rect_dsc_t d;lv_draw_rect_dsc_init(&d);
 d.radius=radius;d.bg_color=dock_color(color);d.bg_opa=border?LV_OPA_TRANSP:LV_OPA_COVER;
 d.border_color=dock_color(color);d.border_width=border;d.border_opa=LV_OPA_COVER;
 lv_area_t area={x,y,x+w-1,y+h-1};lv_draw_rect(layer,&d,&area);
}
static void status_line(lv_layer_t *layer,int x1,int y1,int x2,int y2,unsigned color,int width){
 lv_draw_line_dsc_t d;lv_draw_line_dsc_init(&d);d.p1.x=x1;d.p1.y=y1;d.p2.x=x2;d.p2.y=y2;
 d.color=dock_color(color);d.width=width;d.round_start=d.round_end=1;lv_draw_line(layer,&d);
}
static void status_arc(lv_layer_t *layer,int x,int y,int radius,int start,int end,unsigned color,int width){
 lv_draw_arc_dsc_t d;lv_draw_arc_dsc_init(&d);d.center.x=x;d.center.y=y;d.radius=radius;
 d.start_angle=start;d.end_angle=end;d.color=dock_color(color);d.width=width;d.rounded=1;lv_draw_arc(layer,&d);
}
static void draw_battery(lv_event_t *e){
 lv_area_t a;lv_obj_get_coords(lv_event_get_target_obj(e),&a);lv_layer_t *layer=lv_event_get_layer(e);
 int x=a.x1,y=a.y1;unsigned color=dock_theme_resolve(battery_color(header_battery_level));
 status_rect(layer,x+12,y,8,3,color,0,1);
 status_rect(layer,x+8,y+3,16,29,color,2,3);
 if(header_battery_level>=0){
  int fill=(22*header_battery_level+99)/100;
  if(fill)status_rect(layer,x+11,y+28-fill,10,fill,color,0,1);
  if(header_charging){
   status_line(layer,x+18,y+10,x+14,y+18,0xf8fafc,2);
   status_line(layer,x+14,y+18,x+18,y+18,0xf8fafc,2);
   status_line(layer,x+18,y+18,x+14,y+25,0xf8fafc,2);
  }
 }else status_line(layer,x+11,y+24,x+21,y+10,color,2);
}
static void draw_wifi(lv_event_t *e){
 lv_area_t a;lv_obj_get_coords(lv_event_get_target_obj(e),&a);lv_layer_t *layer=lv_event_get_layer(e);
 int x=a.x1,y=a.y1;
 for(int i=0;i<3;i++)status_arc(layer,x+16,y+28,8+i*7,225,315,dock_theme_resolve(header_wifi_level>=i+2?0xe2e8f0:0x475569),3);
 status_rect(layer,x+14,y+26,4,4,dock_theme_resolve(header_wifi_level?0xe2e8f0:0x64748b),0,2);
 if(!header_wifi_level){
  status_line(layer,x+3,y+3,x+29,y+29,dock_theme_resolve(0x121d2c),5);
  status_line(layer,x+3,y+3,x+29,y+29,dock_theme_resolve(0x94a3b8),2);
 }
}
static void draw_bluetooth(lv_event_t *e){
 lv_area_t a;lv_obj_get_coords(lv_event_get_target_obj(e),&a);lv_layer_t *l=lv_event_get_layer(e);
 int x=a.x1,y=a.y1;unsigned c=dock_theme_resolve(0xe2e8f0);
 switch(header_bluetooth_kind){
 case 2: /* headphones */
  status_arc(l,x+16,y+18,12,180,360,c,2);
  status_line(l,x+5,y+17,x+5,y+26,c,2);status_line(l,x+27,y+17,x+27,y+26,c,2);
  status_rect(l,x+3,y+17,7,12,c,0,2);status_rect(l,x+22,y+17,7,12,c,0,2);break;
 case 3: /* speaker */
  status_rect(l,x+8,y+1,18,30,c,2,3);status_arc(l,x+17,y+22,6,0,360,c,2);
  status_rect(l,x+15,y+6,4,4,c,0,2);break;
 case 4: /* keyboard */
  status_rect(l,x+1,y+6,30,22,c,2,3);
  for(int row=0;row<2;row++)for(int col=0;col<5;col++)status_rect(l,x+5+col*5,y+10+row*5,2,2,c,0,0);
  status_line(l,x+10,y+23,x+23,y+23,c,2);break;
 case 5: /* mouse */
  status_rect(l,x+8,y+1,17,30,c,2,8);status_line(l,x+9,y+12,x+24,y+12,c,2);
  status_line(l,x+16,y+2,x+16,y+12,c,2);break;
 case 6: /* controller */
  status_rect(l,x+1,y+9,30,19,c,2,6);status_line(l,x+7,y+18,x+15,y+18,c,2);
  status_line(l,x+11,y+14,x+11,y+22,c,2);status_rect(l,x+21,y+14,3,3,c,0,2);status_rect(l,x+25,y+20,3,3,c,0,2);break;
 case 7: /* phone */
  status_rect(l,x+8,y+1,17,30,c,2,3);status_line(l,x+14,y+5,x+19,y+5,c,2);
  status_line(l,x+14,y+27,x+19,y+27,c,2);break;
 case 8: /* computer */
  status_rect(l,x+1,y+3,30,21,c,2,2);status_line(l,x+16,y+24,x+16,y+29,c,2);
  status_line(l,x+8,y+29,x+24,y+29,c,2);break;
 case 9: /* watch */
  status_rect(l,x+12,y,8,8,c,0,2);status_rect(l,x+12,y+24,8,8,c,0,2);
  status_rect(l,x+7,y+7,18,18,c,2,5);status_line(l,x+16,y+11,x+16,y+17,c,2);
  status_line(l,x+16,y+17,x+20,y+17,c,2);break;
 case 10: /* camera */
  status_rect(l,x+1,y+8,30,21,c,2,3);status_rect(l,x+8,y+3,10,5,c,0,1);
  status_arc(l,x+17,y+18,7,0,360,c,2);break;
 default: /* connected device with no recognized type */
  status_line(l,x+15,y+1,x+15,y+31,c,2);status_line(l,x+15,y+1,x+25,y+9,c,2);
  status_line(l,x+25,y+9,x+6,y+25,c,2);status_line(l,x+6,y+7,x+25,y+23,c,2);
  status_line(l,x+25,y+23,x+15,y+31,c,2);break;
 }
}
static lv_obj_t *status_icon(lv_obj_t *parent,int x,lv_event_cb_t draw){
 lv_obj_t *obj=lv_obj_create(parent);dock_panel(obj,0x121d2c);
 lv_obj_set_style_bg_opa(obj,LV_OPA_TRANSP,0);lv_obj_set_pos(obj,x,16);lv_obj_set_size(obj,32,32);
 lv_obj_remove_flag(obj,LV_OBJ_FLAG_SCROLLABLE|LV_OBJ_FLAG_CLICKABLE);
 lv_obj_add_event_cb(obj,draw,LV_EVENT_DRAW_MAIN,NULL);return obj;
}
void dock_header_create(lv_obj_t *parent){
 lv_obj_t *header=lv_obj_create(parent);dock_panel(header,0x121d2c);
 lv_obj_set_size(header,640,64);lv_obj_remove_flag(header,LV_OBJ_FLAG_SCROLLABLE);
 dock_label(header,"组件中心",20,20,144,dock_font_body,0xf1f5f9);
 header_clock=dock_label(header,"等待对时",212,20,216,dock_font_body,0xe2e8f0);
 lv_obj_set_style_text_align(header_clock,LV_TEXT_ALIGN_CENTER,0);
 lv_label_set_long_mode(header_clock,LV_LABEL_LONG_CLIP);lv_obj_set_height(header_clock,28);
 header_bluetooth=status_icon(header,486,draw_bluetooth);lv_obj_add_flag(header_bluetooth,LV_OBJ_FLAG_HIDDEN);
 header_wifi=status_icon(header,532,draw_wifi);header_battery=status_icon(header,580,draw_battery);
 header_battery_level=-1;header_charging=header_wifi_level=header_bluetooth_kind=0;
}
void dock_catalog_header(const char *clock,int battery,int charging,int wifi,int bluetooth){
 if(strcmp(lv_label_get_text(header_clock),clock))lv_label_set_text(header_clock,clock);
 if(battery<-1)battery=-1;if(battery>100)battery=100;
 if(wifi<0)wifi=0;if(wifi>4)wifi=4;
 if(header_battery_level!=battery||header_charging!=charging){header_battery_level=battery;header_charging=charging;lv_obj_invalidate(header_battery);}
 if(header_wifi_level!=wifi){header_wifi_level=wifi;lv_obj_invalidate(header_wifi);}
 if(header_bluetooth_kind!=bluetooth){header_bluetooth_kind=bluetooth;
  if(bluetooth)lv_obj_remove_flag(header_bluetooth,LV_OBJ_FLAG_HIDDEN);else lv_obj_add_flag(header_bluetooth,LV_OBJ_FLAG_HIDDEN);
  lv_obj_invalidate(header_bluetooth);
 }
}
