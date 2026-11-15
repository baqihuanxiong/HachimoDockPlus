//go:build lvgl && cgo
#include "bridge.h"
#include "ui_native.h"
#include "page_newapi.h"
#include <stdio.h>
#include <string.h>

#define NEWAPI_MAX_ROWS 100
static lv_obj_t *newapi_screen,*newapi_status,*newapi_refresh,*newapi_reset;
static lv_obj_t *newapi_metrics[3][3],*newapi_balance_names[2],*newapi_balance_values[2],*newapi_balance_notes[2];
static lv_obj_t *newapi_lists[2],*newapi_empty[2],*newapi_rows[2][NEWAPI_MAX_ROWS],*newapi_names[2][NEWAPI_MAX_ROWS],*newapi_values[2][NEWAPI_MAX_ROWS],*newapi_bars[2][NEWAPI_MAX_ROWS];
static int newapi_counts[2];
static lv_obj_t *newapi_overlay,*newapi_count,*newapi_cancel;

static void newapi_visible(lv_obj_t *o,int visible){if(visible)lv_obj_remove_flag(o,LV_OBJ_FLAG_HIDDEN);else lv_obj_add_flag(o,LV_OBJ_FLAG_HIDDEN);}
static void newapi_enabled(lv_obj_t *o,int enabled){if(enabled)lv_obj_remove_state(o,LV_STATE_DISABLED);else lv_obj_add_state(o,LV_STATE_DISABLED);}
static lv_obj_t *newapi_text(lv_obj_t *parent,const char *text,int x,int y,int width,const lv_font_t *font,unsigned fg){lv_obj_t *o=dock_label(parent,text,x,y,width,font,fg);lv_obj_set_height(o,font->line_height+2);lv_label_set_long_mode(o,LV_LABEL_LONG_DOT);lv_obj_remove_flag(o,LV_OBJ_FLAG_CLICKABLE|LV_OBJ_FLAG_SCROLLABLE);return o;}
static lv_obj_t *newapi_panel(int x,int y,int height){lv_obj_t *o=lv_obj_create(newapi_screen);dock_panel(o,0x182438);lv_obj_set_pos(o,x,y);lv_obj_set_size(o,298,height);lv_obj_set_style_radius(o,12,0);lv_obj_remove_flag(o,LV_OBJ_FLAG_SCROLLABLE);return o;}
static void newapi_back(lv_event_t *e){(void)e;if(!lv_obj_has_flag(newapi_overlay,LV_OBJ_FLAG_HIDDEN))dock_emit("newapi-monitor","reset.cancel","");else dock_emit("newapi-monitor","shell.back","");}
int dock_newapi_mount(void){
 memset(newapi_counts,0,sizeof(newapi_counts));
 newapi_screen=lv_obj_create(NULL);dock_panel(newapi_screen,0x0c1220);lv_obj_remove_flag(newapi_screen,LV_OBJ_FLAG_SCROLLABLE);
 lv_obj_t *back=dock_button(newapi_screen,"返回",16,12,78,44,0x1e293b,NULL,NULL);lv_obj_add_event_cb(back,newapi_back,LV_EVENT_CLICKED,NULL);
 newapi_text(newapi_screen,"NewAPI 监控",110,20,240,dock_font_title,0xf1f5f9);
 newapi_refresh=dock_button(newapi_screen,"刷新",370,12,82,44,0x334155,"newapi-monitor","refresh");
 newapi_reset=dock_button(newapi_screen,"重置余额",466,12,158,44,0x0f766e,"newapi-monitor","reset.begin");
 newapi_status=newapi_text(newapi_screen,"正在连接…",18,66,604,dock_font_small,0x94a3b8);
 lv_obj_t *usage=newapi_panel(16,98,168);
 newapi_text(usage,"用量",12,12,44,dock_font_small,0x94a3b8);newapi_text(usage,"消耗",68,12,82,dock_font_small,0x94a3b8);newapi_text(usage,"调用",162,12,52,dock_font_small,0x94a3b8);newapi_text(usage,"成功率",226,12,68,dock_font_small,0x94a3b8);
 const char *periods[]={"今日","本周","本月"};
 for(int i=0;i<3;i++){newapi_text(usage,periods[i],12,48+i*40,48,dock_font_small,0xe2e8f0);newapi_metrics[i][0]=newapi_text(usage,"--",68,48+i*40,88,dock_font_small,0x5eead4);newapi_metrics[i][1]=newapi_text(usage,"--",162,48+i*40,60,dock_font_small,0xe2e8f0);newapi_metrics[i][2]=newapi_text(usage,"--",226,48+i*40,66,dock_font_small,0xe2e8f0);}
 lv_obj_t *upstream=newapi_panel(326,98,168);newapi_text(upstream,"上游余额",12,12,274,dock_font_small,0x94a3b8);
 for(int i=0;i<2;i++){newapi_balance_names[i]=newapi_text(upstream,"",12,44+i*68,140,dock_font_small,0xe2e8f0);newapi_balance_values[i]=newapi_text(upstream,"",152,34+i*68,132,dock_font_title,0x5eead4);lv_obj_set_style_text_align(newapi_balance_values[i],LV_TEXT_ALIGN_RIGHT,0);newapi_balance_notes[i]=newapi_text(upstream,"",12,70+i*68,274,dock_font_small,0x94a3b8);}
 const char *titles[]={"Top 模型 · 7日调用占比","Top 用户 · 7日消耗"};
 for(int k=0;k<2;k++){
  lv_obj_t *panel=newapi_panel(16+k*310,278,190);newapi_text(panel,titles[k],12,10,274,dock_font_small,0x94a3b8);
  newapi_lists[k]=lv_obj_create(panel);dock_panel(newapi_lists[k],0x182438);lv_obj_set_pos(newapi_lists[k],12,40);lv_obj_set_size(newapi_lists[k],274,140);lv_obj_set_scroll_dir(newapi_lists[k],LV_DIR_VER);
  newapi_empty[k]=newapi_text(panel,"暂无调用",12,80,274,dock_font_body,0x94a3b8);
 }
 newapi_overlay=lv_obj_create(newapi_screen);dock_panel(newapi_overlay,0x0c1220);lv_obj_set_size(newapi_overlay,640,480);lv_obj_remove_flag(newapi_overlay,LV_OBJ_FLAG_SCROLLABLE);
 dock_label(newapi_overlay,"重置所有用户余额",40,50,560,dock_font_title,0xf1f5f9);
 newapi_count=dock_label(newapi_overlay,"",40,126,560,dock_font_title,0x5eead4);
 dock_label(newapi_overlay,"SW2 立即执行 · SW3 返回取消\n倒计时结束将自动执行",40,248,560,dock_font_body,0xe2e8f0);
 dock_button(newapi_overlay,"立即执行 SW2",40,352,264,60,0x0f766e,"newapi-monitor","reset.confirm");newapi_cancel=dock_button(newapi_overlay,"取消 SW3",328,352,264,60,0x334155,"newapi-monitor","reset.cancel");
 newapi_visible(newapi_overlay,0);lv_screen_load(newapi_screen);return 0;
}
void dock_newapi_render(const char *status,int busy,int countdown,const char *reset){if(!newapi_screen)return;lv_label_set_text(newapi_status,status);newapi_enabled(newapi_refresh,!busy&&!countdown);newapi_enabled(newapi_reset,!busy&&!countdown);newapi_visible(newapi_overlay,countdown>0);lv_label_set_text(newapi_count,reset);}
void dock_newapi_metric(int index,const char *usd,const char *calls,const char *rate){if(!newapi_screen||index<0||index>2)return;lv_label_set_text(newapi_metrics[index][0],usd);lv_label_set_text(newapi_metrics[index][1],calls);lv_label_set_text(newapi_metrics[index][2],rate);}
void dock_newapi_balance(int index,const char *name,const char *value,const char *note){if(!newapi_screen||index<0||index>1)return;lv_label_set_text(newapi_balance_names[index],name);lv_label_set_text(newapi_balance_values[index],value);lv_label_set_text(newapi_balance_notes[index],note);}
void dock_newapi_rows(int kind,int count){
 if(!newapi_screen||kind<0||kind>1)return;if(count<0)count=0;if(count>NEWAPI_MAX_ROWS)count=NEWAPI_MAX_ROWS;
 for(int i=count;i<newapi_counts[kind];i++){lv_obj_delete(newapi_rows[kind][i]);newapi_rows[kind][i]=newapi_names[kind][i]=newapi_values[kind][i]=newapi_bars[kind][i]=NULL;}
 for(int i=newapi_counts[kind];i<count;i++){
  lv_obj_t *row=lv_obj_create(newapi_lists[kind]);dock_panel(row,0x182438);lv_obj_set_pos(row,0,i*28);lv_obj_set_size(row,274,28);lv_obj_remove_flag(row,LV_OBJ_FLAG_CLICKABLE|LV_OBJ_FLAG_SCROLLABLE);newapi_rows[kind][i]=row;
  newapi_names[kind][i]=newapi_text(row,"",0,0,190,dock_font_small,0xe2e8f0);newapi_values[kind][i]=newapi_text(row,"",194,0,80,dock_font_small,0xe2e8f0);lv_obj_set_style_text_align(newapi_values[kind][i],LV_TEXT_ALIGN_RIGHT,0);
  lv_obj_t *bar=lv_bar_create(row);lv_obj_set_pos(bar,0,23);lv_obj_set_size(bar,274,4);lv_bar_set_range(bar,0,1000);dock_theme_background(bar,0x334155,LV_PART_MAIN);dock_theme_background(bar,0x0f766e,LV_PART_INDICATOR);newapi_bars[kind][i]=bar;
 }
 newapi_counts[kind]=count;newapi_visible(newapi_empty[kind],count==0);
}
void dock_newapi_row(int kind,int index,const char *name,const char *value,int amount){if(!newapi_screen||kind<0||kind>1||index<0||index>=newapi_counts[kind])return;lv_label_set_text(newapi_names[kind][index],name);lv_label_set_text(newapi_values[kind][index],value);lv_bar_set_value(newapi_bars[kind][index],amount,LV_ANIM_OFF);}
void dock_newapi_unmount(void){
 if(newapi_screen)lv_obj_delete(newapi_screen);newapi_screen=newapi_status=newapi_refresh=newapi_reset=newapi_overlay=newapi_count=newapi_cancel=NULL;
 memset(newapi_metrics,0,sizeof(newapi_metrics));memset(newapi_balance_names,0,sizeof(newapi_balance_names));memset(newapi_balance_values,0,sizeof(newapi_balance_values));memset(newapi_balance_notes,0,sizeof(newapi_balance_notes));memset(newapi_lists,0,sizeof(newapi_lists));memset(newapi_empty,0,sizeof(newapi_empty));memset(newapi_rows,0,sizeof(newapi_rows));memset(newapi_names,0,sizeof(newapi_names));memset(newapi_values,0,sizeof(newapi_values));memset(newapi_bars,0,sizeof(newapi_bars));memset(newapi_counts,0,sizeof(newapi_counts));
}
