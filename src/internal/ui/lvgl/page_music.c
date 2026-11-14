//go:build lvgl && cgo
#include "bridge.h"
#include "ui_native.h"
#include "page_music.h"
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

static lv_obj_t *music_screen,*music_status,*music_login,*music_player;
static lv_obj_t *music_title,*music_artist,*music_album,*music_play_label,*music_time,*music_progress;
static lv_obj_t *music_volume_button,*music_volume_overlay,*music_volume,*music_volume_label;
static lv_obj_t *music_library,*music_playlists,*music_tracks,*music_account,*music_logout,*music_library_button,*music_pages;
static lv_obj_t *music_previous_page,*music_next_page;
static lv_obj_t *music_phone,*music_captcha,*music_sms_send,*music_sms_submit,*music_sms_keyboard;
static lv_obj_t *music_art_view,*music_disc,*music_cover_clip,*music_cover_image,*music_fallback,*music_lyrics_view;
static lv_obj_t *music_lyric_previous,*music_lyric_current,*music_lyric_next;
static lv_image_dsc_t music_cover_data;
static unsigned char *music_cover_pixels;
static int music_list_mode,music_busy,music_sms_cooldown,music_logged,music_duration,music_elapsed,music_seeking;

static void music_visible(lv_obj_t *o,int visible){if(visible)lv_obj_remove_flag(o,LV_OBJ_FLAG_HIDDEN);else lv_obj_add_flag(o,LV_OBJ_FLAG_HIDDEN);}
static void music_enabled(lv_obj_t *obj,int enabled){if(enabled)lv_obj_remove_state(obj,LV_STATE_DISABLED);else lv_obj_add_state(obj,LV_STATE_DISABLED);}
static lv_obj_t *music_text(lv_obj_t *parent,const char *text,int x,int y,int width,const lv_font_t *font,unsigned fg){
 lv_obj_t *label=dock_label(parent,text,x,y,width,font,fg);lv_obj_set_height(label,font->line_height+6);lv_label_set_long_mode(label,LV_LABEL_LONG_DOT);return label;
}
static void music_show_lists(lv_event_t *e){
 music_list_mode=(int)(intptr_t)lv_event_get_user_data(e);music_visible(music_library,1);music_visible(music_playlists,!music_list_mode);music_visible(music_tracks,music_list_mode);music_visible(music_pages,music_list_mode);
 music_visible(music_volume_overlay,0);
}
static void music_hide_lists(lv_event_t *e){(void)e;music_visible(music_library,0);}
static void music_row_free(lv_event_t *e){free(lv_event_get_user_data(e));}
static void music_choose(lv_event_t *e){if(music_busy)return;dock_emit("music","playlist.select",lv_event_get_user_data(e));music_list_mode=1;music_visible(music_playlists,0);music_visible(music_tracks,1);music_visible(music_pages,1);}
static void music_choose_track(lv_event_t *e){dock_emit("music","track.play",lv_event_get_user_data(e));music_visible(music_library,0);}
static lv_obj_t *music_list(lv_obj_t *parent){
 lv_obj_t *o=lv_obj_create(parent);dock_panel(o,0x0c1220);lv_obj_set_pos(o,0,60);lv_obj_set_size(o,608,272);lv_obj_set_flex_flow(o,LV_FLEX_FLOW_COLUMN);lv_obj_set_style_pad_row(o,8,0);lv_obj_set_scroll_dir(o,LV_DIR_VER);return o;
}
static void music_volume_toggle(lv_event_t *e){(void)e;music_visible(music_volume_overlay,lv_obj_has_flag(music_volume_overlay,LV_OBJ_FLAG_HIDDEN));lv_obj_move_foreground(music_volume_overlay);}
static void music_volume_close(lv_event_t *e){if(lv_event_get_target(e)==music_volume_overlay)music_visible(music_volume_overlay,0);}
static void music_volume_event(lv_event_t *e){
 char text[24];snprintf(text,sizeof(text),"%d%%",(int)lv_slider_get_value(music_volume));lv_label_set_text(music_volume_label,text);
 if(lv_event_get_code(e)==LV_EVENT_RELEASED){snprintf(text,sizeof(text),"%d",(int)lv_slider_get_value(music_volume));dock_emit("music","volume.set",text);}
}
static void music_time_update(int elapsed){char text[64];snprintf(text,sizeof(text),"%d:%02d / %d:%02d",elapsed/60,elapsed%60,music_duration/60,music_duration%60);lv_label_set_text(music_time,text);}
static void music_seek_event(lv_event_t *e){
 lv_event_code_t code=lv_event_get_code(e);if(code==LV_EVENT_PRESSED)music_seeking=1;
 int position=(int)((long long)lv_slider_get_value(music_progress)*music_duration/1000);
 if(code==LV_EVENT_VALUE_CHANGED&&music_seeking)music_time_update(position);
 if(code==LV_EVENT_RELEASED&&music_seeking){music_seeking=0;char text[24];snprintf(text,sizeof(text),"%d",position);dock_emit("music","play.seek",text);}
 if(code==LV_EVENT_PRESS_LOST){music_seeking=0;music_time_update(music_elapsed);}
}
static void music_visual_toggle(lv_event_t *e){(void)e;int lyrics=lv_obj_has_flag(music_lyrics_view,LV_OBJ_FLAG_HIDDEN);music_visible(music_lyrics_view,lyrics);music_visible(music_art_view,!lyrics);}
static void music_sms_keyboard_hide(void){if(music_sms_keyboard)music_visible(music_sms_keyboard,0);}
static void music_sms_focus(lv_event_t *e){lv_keyboard_set_textarea(music_sms_keyboard,lv_event_get_target(e));music_visible(music_sms_keyboard,1);lv_obj_move_foreground(music_sms_keyboard);}
static void music_sms_send_clicked(lv_event_t *e){(void)e;if(music_busy||music_sms_cooldown)return;dock_emit("music","sms.send",lv_textarea_get_text(music_phone));music_sms_keyboard_hide();}
static void music_sms_submit_clicked(lv_event_t *e){
 (void)e;if(music_busy)return;char payload[160];snprintf(payload,sizeof(payload),"{\"phone\":\"%s\",\"captcha\":\"%s\"}",lv_textarea_get_text(music_phone),lv_textarea_get_text(music_captcha));dock_emit("music","sms.login",payload);memset(payload,0,sizeof(payload));lv_textarea_set_text(music_captcha,"");music_sms_keyboard_hide();
}
static void music_sms_keyboard_event(lv_event_t *e){if(lv_event_get_code(e)==LV_EVENT_CANCEL)music_sms_keyboard_hide();else if(lv_keyboard_get_textarea(music_sms_keyboard)==music_captcha)music_sms_submit_clicked(e);else lv_keyboard_set_textarea(music_sms_keyboard,music_captcha);}
void dock_music_login(int cooldown,int busy){
 music_sms_cooldown=cooldown;music_enabled(music_sms_send,!busy&&cooldown==0);music_enabled(music_sms_submit,!busy);
 char text[48];if(cooldown>0)snprintf(text,sizeof(text),"%d 秒后重发",cooldown);else snprintf(text,sizeof(text),"获取验证码");lv_label_set_text(lv_obj_get_child(music_sms_send,0),text);
 if(music_logged)music_sms_keyboard_hide();
}
static lv_obj_t *music_input(lv_obj_t *parent,int y,int length,const char *placeholder){
 lv_obj_t *o=lv_textarea_create(parent);lv_obj_set_pos(o,24,y);lv_obj_set_size(o,344,52);lv_textarea_set_one_line(o,true);lv_textarea_set_max_length(o,length);lv_textarea_set_accepted_chars(o,"0123456789");lv_textarea_set_placeholder_text(o,placeholder);lv_obj_set_style_text_font(o,dock_font_body,0);dock_theme_foreground(o,0x94a3b8,LV_PART_TEXTAREA_PLACEHOLDER);lv_obj_add_event_cb(o,music_sms_focus,LV_EVENT_FOCUSED,NULL);lv_obj_add_event_cb(o,music_sms_focus,LV_EVENT_CLICKED,NULL);return o;
}
static lv_obj_t *music_lyric_label(int y,int height,const lv_font_t *font,unsigned color){
 lv_obj_t *o=dock_label(music_lyrics_view,"",24,y,560,font,color);lv_obj_set_height(o,height);lv_obj_set_style_text_align(o,LV_TEXT_ALIGN_CENTER,0);lv_obj_remove_flag(o,LV_OBJ_FLAG_CLICKABLE);return o;
}
int dock_music_mount(void){
 music_list_mode=music_busy=music_sms_cooldown=music_logged=music_duration=music_elapsed=music_seeking=0;
 music_cover_image=NULL;music_cover_pixels=NULL;
 music_screen=lv_obj_create(NULL);dock_panel(music_screen,0x0c1220);
 dock_button(music_screen,"返回",16,12,78,44,0x1e293b,"music","shell.back");music_text(music_screen,"网易云音乐",110,16,200,dock_font_title,0xf1f5f9);
 music_account=music_text(music_screen,"",316,22,110,dock_font_small,0x94a3b8);
 music_library_button=dock_button(music_screen,"歌单",434,12,90,44,0x0f766e,NULL,NULL);lv_obj_add_event_cb(music_library_button,music_show_lists,LV_EVENT_CLICKED,(void*)0);
 music_logout=dock_button(music_screen,"退出",534,12,90,44,0x334155,"music","logout");
 music_status=music_text(music_screen,"连接网易云音乐…",18,66,604,dock_font_small,0x5eead4);
 music_login=lv_obj_create(music_screen);dock_panel(music_login,0x182438);lv_obj_set_pos(music_login,16,108);lv_obj_set_size(music_login,608,352);lv_obj_set_style_radius(music_login,18,0);lv_obj_remove_flag(music_login,LV_OBJ_FLAG_SCROLLABLE);
 music_text(music_login,"手机号登录",24,14,560,dock_font_title,0xf1f5f9);
 music_phone=music_input(music_login,70,11,"中国大陆手机号（+86）");music_captcha=music_input(music_login,142,6,"短信验证码");lv_textarea_set_password_mode(music_captcha,true);
 music_sms_send=dock_button(music_login,"获取验证码",382,70,202,52,0x334155,NULL,NULL);lv_obj_add_event_cb(music_sms_send,music_sms_send_clicked,LV_EVENT_CLICKED,NULL);
 music_sms_submit=dock_button(music_login,"登录",382,142,202,52,0x0f766e,NULL,NULL);lv_obj_add_event_cb(music_sms_submit,music_sms_submit_clicked,LV_EVENT_CLICKED,NULL);
 dock_label(music_login,"验证码仅用于本次登录，不会保存",24,218,560,dock_font_body,0x94a3b8);
 music_player=lv_obj_create(music_screen);dock_panel(music_player,0x0c1220);lv_obj_set_pos(music_player,16,108);lv_obj_set_size(music_player,608,364);lv_obj_remove_flag(music_player,LV_OBJ_FLAG_SCROLLABLE);
 music_art_view=lv_obj_create(music_player);dock_panel(music_art_view,0x0c1220);lv_obj_set_size(music_art_view,608,230);lv_obj_remove_flag(music_art_view,LV_OBJ_FLAG_SCROLLABLE);
 music_disc=lv_obj_create(music_art_view);dock_panel(music_disc,0x182438);lv_obj_set_pos(music_disc,8,8);lv_obj_set_size(music_disc,208,208);lv_obj_set_style_radius(music_disc,LV_RADIUS_CIRCLE,0);lv_obj_set_style_border_width(music_disc,2,0);lv_obj_set_style_border_color(music_disc,dock_color(0x334155),0);lv_obj_remove_flag(music_disc,LV_OBJ_FLAG_SCROLLABLE);lv_obj_add_event_cb(music_disc,music_visual_toggle,LV_EVENT_CLICKED,NULL);
 music_cover_clip=lv_obj_create(music_disc);dock_panel(music_cover_clip,0x182438);lv_obj_set_size(music_cover_clip,192,192);lv_obj_center(music_cover_clip);lv_obj_set_style_radius(music_cover_clip,LV_RADIUS_CIRCLE,0);lv_obj_set_style_clip_corner(music_cover_clip,true,0);lv_obj_remove_flag(music_cover_clip,LV_OBJ_FLAG_SCROLLABLE|LV_OBJ_FLAG_CLICKABLE);
 music_fallback=dock_label(music_cover_clip,LV_SYMBOL_AUDIO,0,0,80,dock_font_title,0x5eead4);lv_obj_set_style_text_align(music_fallback,LV_TEXT_ALIGN_CENTER,0);lv_obj_center(music_fallback);
 music_text(music_art_view,"我的音乐",240,2,356,dock_font_small,0x5eead4);
 music_title=music_text(music_art_view,"选择一首喜欢的歌",240,36,356,dock_font_title,0xf1f5f9);
 music_artist=music_text(music_art_view,"从你的网易云歌单开始",240,82,356,dock_font_body,0xe2e8f0);
 music_album=music_text(music_art_view,"",240,120,356,dock_font_small,0x94a3b8);
 music_text(music_art_view,"点击唱盘查看歌词",240,180,356,dock_font_small,0x94a3b8);
 music_lyrics_view=lv_obj_create(music_player);dock_panel(music_lyrics_view,0x0c1220);lv_obj_set_size(music_lyrics_view,608,230);lv_obj_remove_flag(music_lyrics_view,LV_OBJ_FLAG_SCROLLABLE);lv_obj_add_event_cb(music_lyrics_view,music_visual_toggle,LV_EVENT_CLICKED,NULL);
 music_lyric_previous=music_lyric_label(14,52,dock_font_body,0x94a3b8);music_lyric_current=music_lyric_label(78,92,dock_font_title,0x5eead4);music_lyric_next=music_lyric_label(176,48,dock_font_body,0x94a3b8);music_visible(music_lyrics_view,0);
 music_progress=lv_slider_create(music_player);lv_obj_set_pos(music_progress,16,248);lv_obj_set_size(music_progress,576,10);lv_slider_set_range(music_progress,0,1000);lv_obj_set_ext_click_area(music_progress,16);lv_obj_add_event_cb(music_progress,music_seek_event,LV_EVENT_ALL,NULL);
 music_time=music_text(music_player,"0:00 / 0:00",16,268,576,dock_font_small,0x94a3b8);
 dock_button(music_player,LV_SYMBOL_PREV,104,308,80,48,0x334155,"music","track.previous");lv_obj_t *play=dock_button(music_player,LV_SYMBOL_PLAY,200,308,144,48,0x0f766e,"music","play.toggle");music_play_label=lv_obj_get_child(play,0);dock_button(music_player,LV_SYMBOL_NEXT,360,308,80,48,0x334155,"music","track.next");
 music_volume_button=dock_button(music_player,LV_SYMBOL_VOLUME_MAX,536,308,64,48,0x334155,NULL,NULL);lv_obj_add_event_cb(music_volume_button,music_volume_toggle,LV_EVENT_CLICKED,NULL);
 music_library=lv_obj_create(music_screen);dock_panel(music_library,0x0c1220);lv_obj_set_pos(music_library,16,104);lv_obj_set_size(music_library,608,368);lv_obj_remove_flag(music_library,LV_OBJ_FLAG_SCROLLABLE);
 lv_obj_t *lists=dock_button(music_library,"我的歌单",0,0,152,46,0x0f766e,NULL,NULL);lv_obj_add_event_cb(lists,music_show_lists,LV_EVENT_CLICKED,(void*)0);
 lv_obj_t *tracks=dock_button(music_library,"歌曲",162,0,100,46,0x334155,NULL,NULL);lv_obj_add_event_cb(tracks,music_show_lists,LV_EVENT_CLICKED,(void*)1);
 dock_button(music_library,"刷新",272,0,100,46,0x334155,"music","playlists.refresh");lv_obj_t *close=dock_button(music_library,"播放器",458,0,150,46,0x334155,NULL,NULL);lv_obj_add_event_cb(close,music_hide_lists,LV_EVENT_CLICKED,NULL);
 music_playlists=music_list(music_library);music_tracks=music_list(music_library);lv_obj_set_height(music_tracks,244);music_visible(music_tracks,0);
 music_pages=lv_obj_create(music_library);dock_panel(music_pages,0x0c1220);lv_obj_set_pos(music_pages,0,314);lv_obj_set_size(music_pages,608,50);lv_obj_remove_flag(music_pages,LV_OBJ_FLAG_SCROLLABLE);
 music_previous_page=dock_button(music_pages,"上一页",0,0,124,44,0x334155,"music","tracks.previous");music_next_page=dock_button(music_pages,"下一页",484,0,124,44,0x334155,"music","tracks.next");music_text(music_pages,"每页 20 首",200,9,220,dock_font_small,0x94a3b8);
 music_volume_overlay=lv_obj_create(music_screen);dock_panel(music_volume_overlay,0x000000);lv_obj_set_size(music_volume_overlay,640,480);lv_obj_set_style_bg_opa(music_volume_overlay,LV_OPA_40,0);lv_obj_remove_flag(music_volume_overlay,LV_OBJ_FLAG_SCROLLABLE);lv_obj_add_event_cb(music_volume_overlay,music_volume_close,LV_EVENT_CLICKED,NULL);
 lv_obj_t *volume_panel=lv_obj_create(music_volume_overlay);dock_panel(volume_panel,0x182438);lv_obj_set_pos(volume_panel,116,286);lv_obj_set_size(volume_panel,408,136);lv_obj_set_style_radius(volume_panel,18,0);lv_obj_remove_flag(volume_panel,LV_OBJ_FLAG_SCROLLABLE);
 music_text(volume_panel,"播放音量",20,14,260,dock_font_body,0xf1f5f9);music_volume_label=music_text(volume_panel,"60%",308,14,80,dock_font_body,0x5eead4);
 dock_button(volume_panel,LV_SYMBOL_MINUS,12,68,40,44,0x334155,"music","volume.down");dock_button(volume_panel,LV_SYMBOL_PLUS,356,68,40,44,0x334155,"music","volume.up");
 music_volume=lv_slider_create(volume_panel);lv_obj_set_pos(music_volume,72,84);lv_obj_set_size(music_volume,264,14);lv_slider_set_range(music_volume,0,100);lv_obj_set_ext_click_area(music_volume,18);lv_obj_add_event_cb(music_volume,music_volume_event,LV_EVENT_VALUE_CHANGED,NULL);lv_obj_add_event_cb(music_volume,music_volume_event,LV_EVENT_RELEASED,NULL);
 music_sms_keyboard=lv_keyboard_create(music_screen);lv_obj_set_align(music_sms_keyboard,LV_ALIGN_TOP_LEFT);lv_keyboard_set_mode(music_sms_keyboard,LV_KEYBOARD_MODE_NUMBER);lv_obj_set_size(music_sms_keyboard,608,160);lv_obj_set_pos(music_sms_keyboard,16,312);lv_obj_set_style_text_font(music_sms_keyboard,dock_font_body,0);lv_obj_add_event_cb(music_sms_keyboard,music_sms_keyboard_event,LV_EVENT_READY,NULL);lv_obj_add_event_cb(music_sms_keyboard,music_sms_keyboard_event,LV_EVENT_CANCEL,NULL);music_visible(music_sms_keyboard,0);
 music_visible(music_volume_overlay,0);music_visible(music_pages,0);music_visible(music_library,0);lv_screen_load(music_screen);return 0;
}
void dock_music_render(const char *status,const char *account,const char *title,const char *artist,const char *album,int playing,int paused,int loading,int elapsed,int duration,int volume,const char *volume_error,int logged_in,int busy,int seekable){
 music_busy=busy;music_logged=logged_in;music_duration=duration;music_elapsed=elapsed;lv_label_set_text(music_status,*volume_error?volume_error:status);lv_label_set_text(music_account,account);music_visible(music_login,!logged_in);music_visible(music_player,logged_in);music_visible(music_library_button,logged_in);music_visible(music_logout,logged_in);
 if(!logged_in){music_visible(music_library,0);music_visible(music_volume_overlay,0);}
 lv_label_set_text(music_title,*title?title:"选择一首喜欢的歌");lv_label_set_text(music_artist,*artist?artist:"从你的网易云歌单开始");lv_label_set_text(music_album,album);lv_label_set_text(music_play_label,loading?LV_SYMBOL_REFRESH:playing&&!paused?LV_SYMBOL_PAUSE:LV_SYMBOL_PLAY);
 if(!seekable)music_seeking=0;music_enabled(music_progress,seekable);if(!music_seeking){lv_slider_set_value(music_progress,duration>0?(int)((long long)elapsed*1000/duration):0,LV_ANIM_OFF);music_time_update(elapsed);}
 if(!lv_obj_has_state(music_volume,LV_STATE_PRESSED)){lv_slider_set_value(music_volume,volume,LV_ANIM_OFF);char text[24];snprintf(text,sizeof(text),"%d%%",volume);lv_label_set_text(music_volume_label,text);}
 lv_label_set_text(lv_obj_get_child(music_volume_button,0),volume==0?LV_SYMBOL_MUTE:LV_SYMBOL_VOLUME_MAX);
}
static void music_cover_release(void){
 if(music_cover_image){lv_obj_delete(music_cover_image);music_cover_image=NULL;}
 if(music_cover_pixels){lv_image_cache_drop(&music_cover_data);free(music_cover_pixels);music_cover_pixels=NULL;}
}
void dock_music_cover(const unsigned char *pixels,int width,int height){
 music_cover_release();music_visible(music_fallback,1);if(!pixels||width<1||height<1||width>512||height>512)return;
 size_t size=(size_t)width*height*4;music_cover_pixels=malloc(size);if(!music_cover_pixels)return;memcpy(music_cover_pixels,pixels,size);
 music_cover_data=(lv_image_dsc_t){.header={.magic=LV_IMAGE_HEADER_MAGIC,.cf=LV_COLOR_FORMAT_ARGB8888,.w=width,.h=height,.stride=width*4},.data_size=size,.data=music_cover_pixels};
 music_cover_image=lv_image_create(music_cover_clip);lv_image_set_src(music_cover_image,&music_cover_data);lv_obj_set_size(music_cover_image,192,192);lv_image_set_inner_align(music_cover_image,LV_IMAGE_ALIGN_CONTAIN);lv_image_set_antialias(music_cover_image,true);lv_obj_center(music_cover_image);lv_obj_remove_flag(music_cover_image,LV_OBJ_FLAG_CLICKABLE);music_visible(music_fallback,0);
}
void dock_music_lyrics(const char *previous,const char *current,const char *next){lv_label_set_text(music_lyric_previous,previous);lv_label_set_text(music_lyric_current,current);lv_label_set_text(music_lyric_next,next);}
void dock_music_lists_clear(void){lv_obj_clean(music_playlists);lv_obj_clean(music_tracks);}
void dock_music_playlist(const char *id,const char *name,int count,int selected){
 char *data=strdup(id);if(!data)return;lv_obj_t *row=dock_button(music_playlists,"",0,0,596,72,selected?0x0f766e:0x182438,NULL,NULL);lv_obj_set_style_pad_all(row,0,0);lv_obj_add_event_cb(row,music_choose,LV_EVENT_CLICKED,data);lv_obj_add_event_cb(row,music_row_free,LV_EVENT_DELETE,data);
 lv_obj_t *label=lv_obj_get_child(row,0);lv_label_set_text(label,name);lv_obj_set_align(label,LV_ALIGN_TOP_LEFT);lv_obj_set_pos(label,16,20);lv_obj_set_size(label,452,32);lv_obj_set_style_text_align(label,LV_TEXT_ALIGN_LEFT,0);lv_label_set_long_mode(label,LV_LABEL_LONG_DOT);dock_theme_foreground(label,selected?0xffffff:0xf1f5f9,0);
 char text[40];snprintf(text,sizeof(text),"%d 首",count);music_text(row,text,484,22,96,dock_font_small,selected?0xffffff:0x94a3b8);
}
void dock_music_track(int index,const char *title,const char *artist,int selected){
 char id[24];snprintf(id,sizeof(id),"%d",index);char *data=strdup(id);if(!data)return;
 lv_obj_t *row=dock_button(music_tracks,"",0,0,596,70,selected?0x0f766e:0x182438,NULL,NULL);lv_obj_set_style_pad_all(row,0,0);lv_obj_add_event_cb(row,music_choose_track,LV_EVENT_CLICKED,data);lv_obj_add_event_cb(row,music_row_free,LV_EVENT_DELETE,data);
 music_text(row,title,16,4,564,dock_font_body,selected?0xffffff:0xf1f5f9);music_text(row,artist,16,36,564,dock_font_small,selected?0xffffff:0x94a3b8);
}
void dock_music_tracks_page(int page,int count,int total){
 char text[80];snprintf(text,sizeof(text),"%d / %d 页 · %d 首",page+1,count>0?count:1,total);lv_label_set_text(lv_obj_get_child(music_pages,2),text);music_enabled(music_previous_page,page>0);music_enabled(music_next_page,page+1<count);
}
int dock_music_back(void){
 if(music_sms_keyboard&&!lv_obj_has_flag(music_sms_keyboard,LV_OBJ_FLAG_HIDDEN)){music_sms_keyboard_hide();return 1;}
 if(music_volume_overlay&&!lv_obj_has_flag(music_volume_overlay,LV_OBJ_FLAG_HIDDEN)){music_visible(music_volume_overlay,0);return 1;}
 if(music_library&&!lv_obj_has_flag(music_library,LV_OBJ_FLAG_HIDDEN)){music_visible(music_library,0);return 1;}return 0;
}
void dock_music_unmount(void){music_cover_release();if(music_screen){lv_obj_delete(music_screen);music_screen=NULL;music_library=NULL;music_sms_keyboard=NULL;music_volume_overlay=NULL;}}
