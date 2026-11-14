//go:build lvgl && cgo
#include "bridge.h"
#include "ui_native.h"
#include "page_settings.h"
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

static lv_obj_t *settings_screen,*settings_status,*wifi_status,*bt_status,*wifi_panel,*bt_panel,*appearance_panel,*audio_panel,*wifi_list,*bt_list;
static lv_obj_t *bt_power,*bt_power_text,*wifi_scan,*bt_scan,*wifi_disconnect,*wifi_manual,*wifi_tab,*bt_tab,*appearance_tab,*audio_tab;
static lv_obj_t *audio_slider,*audio_value,*audio_error;
static lv_obj_t *saver_clock_button,*saver_blank_button;
static lv_obj_t *join_dialog,*join_ssid,*join_password,*join_keyboard;
static int settings_busy,settings_bt_on;
static char join_security[32];
typedef struct{lv_obj_t *button,*check;char id[24],title[48];}theme_choice;
static theme_choice theme_choices[16];
static unsigned theme_choice_count;
static lv_obj_t *theme_selected,*mode_light_button,*mode_dark_button;
static void saver_clicked(lv_event_t *e){if(settings_busy)return;dock_emit("settings","screensaver.set",lv_event_get_user_data(e));}
static void mode_clicked(lv_event_t *e){if(settings_busy)return;dock_emit("settings","mode.set",lv_event_get_user_data(e));}
static void theme_clicked(lv_event_t *e){if(settings_busy)return;theme_choice *choice=lv_event_get_user_data(e);dock_emit("settings","theme.set",choice->id);}

typedef struct {char id[32],ssid[160],security[32],address[24];int paired,connected;} setting_row;
static void row_deleted(lv_event_t *e){free(lv_event_get_user_data(e));}
static void set_enabled(lv_obj_t *obj,int enabled){if(enabled)lv_obj_remove_state(obj,LV_STATE_DISABLED);else lv_obj_add_state(obj,LV_STATE_DISABLED);}
static void close_join(void){if(join_dialog){lv_obj_delete(join_dialog);join_dialog=NULL;join_ssid=NULL;join_password=NULL;join_keyboard=NULL;}}
static void close_join_event(lv_event_t *e){(void)e;close_join();}
static void settings_input_focus(lv_event_t *e){lv_keyboard_set_textarea(join_keyboard,lv_event_get_target(e));}
static int json_string(char *out,size_t cap,const char *in){size_t n=0;if(cap<3)return 0;out[n++]='"';for(;*in;in++){unsigned char c=*in;if(c<32)return 0;if(c=='"'||c=='\\'){if(n+2>=cap)return 0;out[n++]='\\';}if(n+1>=cap)return 0;out[n++]=c;}out[n++]='"';out[n]=0;return 1;}
static void submit_join(lv_event_t *e){(void)e;if(settings_busy)return;char ssid[300],password[300],payload[800];
 if(!json_string(ssid,sizeof(ssid),lv_textarea_get_text(join_ssid))||!json_string(password,sizeof(password),lv_textarea_get_text(join_password)))return;
 snprintf(payload,sizeof(payload),"{\"SSID\":%s,\"Password\":%s,\"Security\":\"%s\"}",ssid,password,join_security);
 dock_emit("settings","wifi.join",payload);lv_textarea_set_text(join_password,"");memset(password,0,sizeof(password));memset(payload,0,sizeof(payload));close_join();
}
static void open_join(const char *ssid,const char *security){
 close_join();snprintf(join_security,sizeof(join_security),"%s",security);
 join_dialog=lv_obj_create(settings_screen);dock_panel(join_dialog,0x182438);lv_obj_set_pos(join_dialog,10,4);lv_obj_set_size(join_dialog,620,472);lv_obj_set_style_radius(join_dialog,12,0);lv_obj_remove_flag(join_dialog,LV_OBJ_FLAG_SCROLLABLE);
 dock_label(join_dialog,"连接 Wi-Fi",18,10,400,dock_font_title,0xf1f5f9);
 join_ssid=lv_textarea_create(join_dialog);lv_obj_set_pos(join_ssid,16,52);lv_obj_set_size(join_ssid,588,48);lv_textarea_set_one_line(join_ssid,true);lv_textarea_set_max_length(join_ssid,32);lv_textarea_set_placeholder_text(join_ssid,"网络名称（SSID）");lv_textarea_set_text(join_ssid,ssid);lv_obj_set_style_text_font(join_ssid,dock_font_body,0);lv_obj_add_event_cb(join_ssid,settings_input_focus,LV_EVENT_FOCUSED,NULL);
 join_password=lv_textarea_create(join_dialog);lv_obj_set_pos(join_password,16,110);lv_obj_set_size(join_password,588,48);lv_textarea_set_one_line(join_password,true);lv_textarea_set_password_mode(join_password,true);lv_textarea_set_max_length(join_password,64);lv_textarea_set_placeholder_text(join_password,"密码");lv_obj_set_style_text_font(join_password,dock_font_body,0);lv_obj_add_event_cb(join_password,settings_input_focus,LV_EVENT_FOCUSED,NULL);
 lv_obj_t *submit=dock_button(join_dialog,"连接并保存",16,168,280,44,0x0f766e,NULL,NULL);lv_obj_add_event_cb(submit,submit_join,LV_EVENT_CLICKED,NULL);
 lv_obj_t *cancel=dock_button(join_dialog,"取消",312,168,292,44,0x334155,NULL,NULL);lv_obj_add_event_cb(cancel,close_join_event,LV_EVENT_CLICKED,NULL);
 dock_label(join_dialog,!strcmp(security,"开放")?"开放网络，无需密码":"支持 WPA/WPA2 Personal",18,220,584,dock_font_small,0x94a3b8);
 join_keyboard=lv_keyboard_create(join_dialog);lv_obj_set_align(join_keyboard,LV_ALIGN_TOP_LEFT);lv_obj_set_size(join_keyboard,620,210);lv_obj_set_pos(join_keyboard,0,262);lv_obj_set_style_text_font(join_keyboard,dock_font_body,0);lv_keyboard_set_textarea(join_keyboard,*ssid?join_password:join_ssid);
 lv_obj_add_event_cb(join_keyboard,submit_join,LV_EVENT_READY,NULL);lv_obj_add_event_cb(join_keyboard,close_join_event,LV_EVENT_CANCEL,NULL);
 lv_obj_move_foreground(join_dialog);
}
static void settings_command(lv_event_t *e){if(settings_busy)return;const char *action=lv_event_get_user_data(e);dock_emit("settings",action,NULL);}
static void settings_power(lv_event_t *e){(void)e;if(!settings_busy)dock_emit("settings",settings_bt_on?"bt.off":"bt.on",NULL);}
static void manual_join(lv_event_t *e){(void)e;if(!settings_busy)open_join("","WPA-PSK");}
static void network_clicked(lv_event_t *e){if(settings_busy)return;setting_row *r=lv_event_get_user_data(e);if(*r->id)dock_emit("settings","wifi.connect",r->id);else open_join(r->ssid,r->security);}
static void network_forget(lv_event_t *e){if(settings_busy)return;setting_row *r=lv_event_get_user_data(e);dock_emit("settings","wifi.forget",r->id);}
static void device_clicked(lv_event_t *e){if(settings_busy)return;setting_row *r=lv_event_get_user_data(e);dock_emit("settings",r->connected?"bt.disconnect":r->paired?"bt.connect":"bt.pair",r->address);}
static void device_forget(lv_event_t *e){if(settings_busy)return;setting_row *r=lv_event_get_user_data(e);dock_emit("settings","bt.remove",r->address);}
static void tab_clicked(lv_event_t *e){
 unsigned selected=(unsigned)(uintptr_t)lv_event_get_user_data(e);
 lv_obj_t *tabs[]={wifi_tab,bt_tab,appearance_tab,audio_tab},*panels[]={wifi_panel,bt_panel,appearance_panel,audio_panel};
 for(unsigned i=0;i<4;i++){dock_theme_button(tabs[i],i==selected);if(i==selected)lv_obj_remove_flag(panels[i],LV_OBJ_FLAG_HIDDEN);else lv_obj_add_flag(panels[i],LV_OBJ_FLAG_HIDDEN);}
}
static lv_obj_t *settings_list(lv_obj_t *parent){lv_obj_t *list=lv_obj_create(parent);dock_panel(list,0x0c1220);lv_obj_set_pos(list,0,92);lv_obj_set_size(list,608,270);lv_obj_set_scroll_dir(list,LV_DIR_VER);lv_obj_set_style_pad_all(list,0,0);lv_obj_set_flex_flow(list,LV_FLEX_FLOW_COLUMN);lv_obj_set_style_pad_row(list,8,0);return list;}
static void settings_volume_event(lv_event_t *e){char text[24];snprintf(text,sizeof(text),"%d%%",(int)lv_slider_get_value(audio_slider));lv_label_set_text(audio_value,text);if(lv_event_get_code(e)==LV_EVENT_RELEASED){snprintf(text,sizeof(text),"%d",(int)lv_slider_get_value(audio_slider));dock_emit("settings","volume.set",text);}}
void dock_settings_volume(int volume,const char *error){if(!lv_obj_has_state(audio_slider,LV_STATE_PRESSED)){lv_slider_set_value(audio_slider,volume,LV_ANIM_OFF);char text[24];snprintf(text,sizeof(text),"%d%%",volume);lv_label_set_text(audio_value,text);}lv_label_set_text(audio_error,error);}
int dock_settings_mount(void){
 settings_busy=1;join_dialog=NULL;theme_choice_count=0;settings_screen=lv_obj_create(NULL);dock_panel(settings_screen,0x0c1220);
 dock_button(settings_screen,"返回",16,12,78,44,0x1e293b,"settings","shell.back");dock_label(settings_screen,"设置",108,16,90,dock_font_title,0xf1f5f9);
 lv_obj_t *w=wifi_tab=dock_button(settings_screen,"Wi-Fi",214,12,98,44,0x0f766e,NULL,NULL);lv_obj_add_event_cb(w,tab_clicked,LV_EVENT_CLICKED,(void*)0);
 lv_obj_t *b=bt_tab=dock_button(settings_screen,"蓝牙",318,12,98,44,0x334155,NULL,NULL);lv_obj_add_event_cb(b,tab_clicked,LV_EVENT_CLICKED,(void*)1);
 lv_obj_t *a=appearance_tab=dock_button(settings_screen,"外观",422,12,98,44,0x334155,NULL,NULL);lv_obj_add_event_cb(a,tab_clicked,LV_EVENT_CLICKED,(void*)2);
 audio_tab=dock_button(settings_screen,"音频",526,12,98,44,0x334155,NULL,NULL);lv_obj_add_event_cb(audio_tab,tab_clicked,LV_EVENT_CLICKED,(void*)3);
 settings_status=dock_label(settings_screen,"读取连接状态…",18,65,604,dock_font_small,0x5eead4);lv_obj_set_height(settings_status,34);lv_label_set_long_mode(settings_status,LV_LABEL_LONG_DOT);
 wifi_panel=lv_obj_create(settings_screen);dock_panel(wifi_panel,0x0c1220);lv_obj_set_pos(wifi_panel,16,106);lv_obj_set_size(wifi_panel,608,366);
 bt_panel=lv_obj_create(settings_screen);dock_panel(bt_panel,0x0c1220);lv_obj_set_pos(bt_panel,16,106);lv_obj_set_size(bt_panel,608,366);lv_obj_add_flag(bt_panel,LV_OBJ_FLAG_HIDDEN);
 wifi_status=dock_label(wifi_panel,"",0,0,608,dock_font_body,0xe2e8f0);lv_obj_set_height(wifi_status,30);lv_label_set_long_mode(wifi_status,LV_LABEL_LONG_DOT);
 wifi_scan=dock_button(wifi_panel,"扫描",0,38,170,44,0x0f766e,NULL,NULL);lv_obj_add_event_cb(wifi_scan,settings_command,LV_EVENT_CLICKED,"wifi.scan");
 wifi_manual=dock_button(wifi_panel,"手动添加",186,38,218,44,0x334155,NULL,NULL);lv_obj_add_event_cb(wifi_manual,manual_join,LV_EVENT_CLICKED,NULL);
 wifi_disconnect=dock_button(wifi_panel,"断开",420,38,188,44,0x334155,NULL,NULL);lv_obj_add_event_cb(wifi_disconnect,settings_command,LV_EVENT_CLICKED,"wifi.disconnect");wifi_list=settings_list(wifi_panel);
 bt_status=dock_label(bt_panel,"",0,0,608,dock_font_body,0xe2e8f0);lv_obj_set_height(bt_status,30);lv_label_set_long_mode(bt_status,LV_LABEL_LONG_DOT);
 bt_power=dock_button(bt_panel,"开启蓝牙",0,38,200,44,0x334155,NULL,NULL);bt_power_text=lv_obj_get_child(bt_power,0);lv_obj_add_event_cb(bt_power,settings_power,LV_EVENT_CLICKED,NULL);
 bt_scan=dock_button(bt_panel,"扫描设备",216,38,200,44,0x0f766e,NULL,NULL);lv_obj_add_event_cb(bt_scan,settings_command,LV_EVENT_CLICKED,"bt.scan");bt_list=settings_list(bt_panel);
 dock_label(bt_panel,"支持无需输入 PIN 的设备配对",0,86,608,dock_font_small,0x94a3b8);lv_obj_set_y(bt_list,116);lv_obj_set_height(bt_list,246);
 appearance_panel=lv_obj_create(settings_screen);dock_panel(appearance_panel,0x0c1220);lv_obj_set_pos(appearance_panel,16,106);lv_obj_set_size(appearance_panel,608,366);lv_obj_remove_flag(appearance_panel,LV_OBJ_FLAG_SCROLLABLE);lv_obj_add_flag(appearance_panel,LV_OBJ_FLAG_HIDDEN);
 dock_label(appearance_panel,"显示模式",0,10,152,dock_font_body,0xe2e8f0);
 mode_light_button=dock_button(appearance_panel,"白天模式",168,0,204,44,0x334155,NULL,NULL);lv_obj_add_event_cb(mode_light_button,mode_clicked,LV_EVENT_CLICKED,"light");
 mode_dark_button=dock_button(appearance_panel,"黑夜模式",388,0,220,44,0x0f766e,NULL,NULL);lv_obj_add_event_cb(mode_dark_button,mode_clicked,LV_EVENT_CLICKED,"dark");
 theme_selected=dock_label(appearance_panel,"主题色",0,56,608,dock_font_body,0xe2e8f0);
 dock_label(appearance_panel,"屏保模式",0,286,152,dock_font_body,0xe2e8f0);
 saver_clock_button=dock_button(appearance_panel,"时钟屏保",168,276,204,44,0x0f766e,NULL,NULL);lv_obj_add_event_cb(saver_clock_button,saver_clicked,LV_EVENT_CLICKED,"clock");
 saver_blank_button=dock_button(appearance_panel,"纯黑屏保",388,276,220,44,0x334155,NULL,NULL);lv_obj_add_event_cb(saver_blank_button,saver_clicked,LV_EVENT_CLICKED,"blank");
 dock_label(appearance_panel,"组件中心按 SW3 切换屏保，选择自动保存",0,338,608,dock_font_small,0x94a3b8);
 audio_panel=lv_obj_create(settings_screen);dock_panel(audio_panel,0x0c1220);lv_obj_set_pos(audio_panel,16,106);lv_obj_set_size(audio_panel,608,366);lv_obj_add_flag(audio_panel,LV_OBJ_FLAG_HIDDEN);
 dock_label(audio_panel,"全局播放音量",12,16,380,dock_font_title,0xf1f5f9);
 audio_value=dock_label(audio_panel,"60%",462,20,130,dock_font_title,0x5eead4);
 audio_slider=lv_slider_create(audio_panel);lv_obj_set_pos(audio_slider,28,102);lv_obj_set_size(audio_slider,552,18);lv_slider_set_range(audio_slider,0,100);lv_obj_set_ext_click_area(audio_slider,18);lv_obj_add_event_cb(audio_slider,settings_volume_event,LV_EVENT_VALUE_CHANGED,NULL);lv_obj_add_event_cb(audio_slider,settings_volume_event,LV_EVENT_RELEASED,NULL);
 dock_button(audio_panel,LV_SYMBOL_MINUS,12,150,110,52,0x334155,"settings","volume.down");dock_button(audio_panel,LV_SYMBOL_PLUS,486,150,110,52,0x0f766e,"settings","volume.up");
 dock_label(audio_panel,"同时作用于音乐和语音播放，自动保存",12,226,584,dock_font_body,0xe2e8f0);
 dock_label(audio_panel,"0% 静音 · 100% 对应已验证的最大使用音量",12,264,584,dock_font_small,0x94a3b8);
 audio_error=dock_label(audio_panel,"",12,306,584,dock_font_small,0xfbbf24);
 lv_screen_load(settings_screen);return 0;
}
void dock_settings_render(const char *status,const char *wifi,const char *bt,int on,int ready,int busy){
 settings_busy=busy;settings_bt_on=on;lv_label_set_text(settings_status,status);lv_label_set_text(wifi_status,wifi);lv_label_set_text(bt_status,bt);lv_label_set_text(bt_power_text,on?"关闭蓝牙":"开启蓝牙");
 set_enabled(wifi_scan,!busy);set_enabled(wifi_manual,!busy);set_enabled(wifi_disconnect,!busy);set_enabled(bt_power,!busy&&ready);set_enabled(bt_scan,!busy&&on);set_enabled(wifi_list,!busy);set_enabled(bt_list,!busy);for(unsigned i=0;i<theme_choice_count;i++)set_enabled(theme_choices[i].button,!busy);set_enabled(mode_light_button,!busy);set_enabled(mode_dark_button,!busy);set_enabled(saver_clock_button,!busy);set_enabled(saver_blank_button,!busy);
}
void dock_settings_clear(void){lv_obj_clean(wifi_list);lv_obj_clean(bt_list);}
static lv_obj_t *settings_row(lv_obj_t *list,setting_row *data){lv_obj_t *row=lv_obj_create(list);dock_panel(row,0x182438);lv_obj_set_size(row,594,92);lv_obj_set_style_radius(row,10,0);lv_obj_add_event_cb(row,row_deleted,LV_EVENT_DELETE,data);return row;}
void dock_settings_network(const char *id,const char *ssid,const char *security,int signal,int current){
 setting_row *r=calloc(1,sizeof(*r));if(!r)return;snprintf(r->id,sizeof(r->id),"%s",id);snprintf(r->ssid,sizeof(r->ssid),"%s",ssid);snprintf(r->security,sizeof(r->security),"%s",security);lv_obj_t *row=settings_row(wifi_list,r);
 lv_obj_t *label=dock_label(row,ssid,12,9,340,dock_font_body,0xf1f5f9);lv_obj_set_height(label,28);lv_label_set_long_mode(label,LV_LABEL_LONG_DOT);
 char detail[160];if(signal)snprintf(detail,sizeof(detail),"%s%s · %d dBm",current?"已连接 · ":"",security,signal);else snprintf(detail,sizeof(detail),"%s%s",current?"已连接 · ":"",security);dock_label(row,detail,12,49,340,dock_font_small,0x94a3b8);
 lv_obj_t *connect=dock_button(row,current?"重连":"连接",364,10,100,68,0x0f766e,NULL,NULL);lv_obj_add_event_cb(connect,network_clicked,LV_EVENT_CLICKED,r);if(!strcmp(security,"不支持"))set_enabled(connect,0);
 if(*id){lv_obj_t *forget=dock_button(row,"忘记",476,10,106,68,0x334155,NULL,NULL);lv_obj_add_event_cb(forget,network_forget,LV_EVENT_CLICKED,r);}
}
void dock_settings_device(const char *address,const char *name,int paired,int connected){
 setting_row *r=calloc(1,sizeof(*r));if(!r)return;snprintf(r->address,sizeof(r->address),"%s",address);r->paired=paired;r->connected=connected;lv_obj_t *row=settings_row(bt_list,r);
 lv_obj_t *label=dock_label(row,name,12,8,340,dock_font_body,0xf1f5f9);lv_obj_set_height(label,28);lv_label_set_long_mode(label,LV_LABEL_LONG_DOT);
 char detail[120];snprintf(detail,sizeof(detail),"%s · %s",address,connected?"已连接":paired?"已配对":"未配对");dock_label(row,detail,12,49,340,dock_font_small,0x94a3b8);
 lv_obj_t *connect=dock_button(row,connected?"断开":paired?"连接":"配对",364,10,100,68,0x0f766e,NULL,NULL);lv_obj_add_event_cb(connect,device_clicked,LV_EVENT_CLICKED,r);
 if(paired){lv_obj_t *forget=dock_button(row,"移除",476,10,106,68,0x334155,NULL,NULL);lv_obj_add_event_cb(forget,device_forget,LV_EVENT_CLICKED,r);}
}
void dock_settings_unmount(void){close_join();if(settings_screen){lv_obj_delete(settings_screen);settings_screen=NULL;}}

void dock_settings_theme_add(const char *id,const char *title,unsigned color){
 if(theme_choice_count>=16)return;unsigned index=theme_choice_count++;theme_choice *choice=&theme_choices[index];
 snprintf(choice->id,sizeof(choice->id),"%s",id);snprintf(choice->title,sizeof(choice->title),"%s",title);
 choice->button=dock_button(appearance_panel,title,(index%3)*208,92+(index/3)*56,192,48,color,NULL,NULL);
 lv_obj_set_style_bg_color(choice->button,dock_color(color),0);
 lv_obj_set_style_bg_color(choice->button,dock_color(color),LV_STATE_CHECKED);
 lv_obj_set_style_border_color(choice->button,dock_color(0xffffff),LV_STATE_CHECKED);lv_obj_set_style_border_width(choice->button,3,LV_STATE_CHECKED);
 lv_obj_add_event_cb(choice->button,theme_clicked,LV_EVENT_CLICKED,choice);
 choice->check=dock_label(choice->button,LV_SYMBOL_OK,142,4,32,dock_font_body,0xffffff);lv_obj_add_flag(choice->check,LV_OBJ_FLAG_HIDDEN);
}
void dock_settings_theme(const char *id){
 for(unsigned i=0;i<theme_choice_count;i++){
  theme_choice *choice=&theme_choices[i];if(!strcmp(choice->id,id)){
   lv_obj_add_state(choice->button,LV_STATE_CHECKED);lv_obj_remove_flag(choice->check,LV_OBJ_FLAG_HIDDEN);
   char text[96];snprintf(text,sizeof(text),"主题色 · %s",choice->title);lv_label_set_text(theme_selected,text);
  }else{lv_obj_remove_state(choice->button,LV_STATE_CHECKED);lv_obj_add_flag(choice->check,LV_OBJ_FLAG_HIDDEN);}
 }
}

void dock_settings_mode(const char *mode){
 int light=!strcmp(mode,"light");dock_theme_button(mode_light_button,light);dock_theme_button(mode_dark_button,!light);
 lv_obj_t *light_label=lv_obj_get_child(mode_light_button,0),*dark_label=lv_obj_get_child(mode_dark_button,0);
 dock_theme_foreground(light_label,light?0xffffff:0xe2e8f0,0);dock_theme_foreground(dark_label,light?0xe2e8f0:0xffffff,0);
}

int dock_settings_back(void){if(join_dialog){close_join();return 1;}return 0;}
void dock_settings_screensaver(const char *id){int clock=!strcmp(id,"clock");dock_theme_button(saver_clock_button,clock);dock_theme_button(saver_blank_button,!clock);}
