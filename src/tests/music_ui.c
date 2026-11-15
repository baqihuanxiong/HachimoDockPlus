/* Exercise real LVGL rendering, input events and repeated page lifetimes. */
#include <assert.h>
#include "../internal/ui/lvgl/bridge.c"
#include "../internal/ui/lvgl/statusbar.c"
#include "../internal/ui/lvgl/theme.c"
#include "../internal/ui/lvgl/catalog.c"
#include "../internal/ui/lvgl/screensaver.c"
#include "../internal/ui/lvgl/page_settings.c"
#include "../internal/ui/lvgl/page_music.c"
static void screenshot(const char *path){lv_refr_now(display);const unsigned char *p=dock_pixels();FILE *f=fopen(path,"wb");assert(f);fprintf(f,"P6\n640 480\n255\n");for(int i=0;i<640*480;i++){fputc(p[i*4+2],f);fputc(p[i*4+1],f);fputc(p[i*4],f);}fclose(f);}
int main(int argc,char **argv){
 assert(argc==2);assert(dock_init(NULL,NULL,argv[1])==0);dock_event event;
 for(int iteration=0;iteration<3;iteration++){
  assert(dock_music_mount()==0);
  dock_music_render("请输入手机号，获取短信验证码","","","","",0,0,0,0,0,60,"",0,0,0);dock_music_login(0,0);
  assert(!lv_obj_has_flag(music_login,LV_OBJ_FLAG_HIDDEN));screenshot("build/native/music-login.ppm");
  lv_textarea_set_text(music_phone,"13800138000");lv_obj_send_event(music_phone,LV_EVENT_CLICKED,NULL);assert(!lv_obj_has_flag(music_sms_keyboard,LV_OBJ_FLAG_HIDDEN));
  lv_obj_update_layout(music_screen);lv_area_t code,key;lv_obj_get_coords(music_captcha,&code);lv_obj_get_coords(music_sms_keyboard,&key);assert(code.y2<key.y1&&key.y2<480);assert(dock_music_back()==1);
  lv_obj_send_event(music_sms_send,LV_EVENT_CLICKED,NULL);assert(dock_poll(&event)==1&&!strcmp(event.action,"sms.send"));dock_music_login(60,0);assert(lv_obj_has_state(music_sms_send,LV_STATE_DISABLED));
  lv_textarea_set_text(music_captcha,"123456");lv_obj_send_event(music_sms_submit,LV_EVENT_CLICKED,NULL);assert(dock_poll(&event)==1&&!strcmp(event.action,"sms.login"));assert(!*lv_textarea_get_text(music_captcha));
  dock_music_render("正在播放","我的音乐","示例歌曲","示例歌手","示例专辑",1,0,0,67,191,60,"",1,0,1);dock_music_login(0,0);
  assert(!strcmp(lv_label_get_text(music_play_label),LV_SYMBOL_PAUSE));assert(!strcmp(lv_label_get_text(music_time),"1:07 / 3:11"));assert(lv_obj_has_flag(music_volume_overlay,LV_OBJ_FLAG_HIDDEN));
  unsigned char cover[64*64*4];for(int y=0;y<64;y++)for(int x=0;x<64;x++){int i=(y*64+x)*4;cover[i]=50+x*2;cover[i+1]=30+y*3;cover[i+2]=190-x;cover[i+3]=255;}dock_music_cover(cover,64,64);assert(music_cover_pixels&&music_cover_pixels!=cover);assert(lv_obj_has_flag(music_fallback,LV_OBJ_FLAG_HIDDEN));
  screenshot("build/native/music-player.ppm");dock_theme_appearance(0x2563eb,1);screenshot("build/native/music-player-light.ppm");dock_theme_appearance(0x0f766e,0);
  lv_obj_send_event(music_disc,LV_EVENT_CLICKED,NULL);dock_music_lyrics("示例前一行","随音乐显示的示例歌词","点击歌词返回封面唱盘");assert(!lv_obj_has_flag(music_lyrics_view,LV_OBJ_FLAG_HIDDEN));screenshot("build/native/music-lyrics.ppm");lv_obj_send_event(music_lyrics_view,LV_EVENT_CLICKED,NULL);assert(!lv_obj_has_flag(music_art_view,LV_OBJ_FLAG_HIDDEN));
  lv_obj_send_event(music_progress,LV_EVENT_PRESSED,NULL);lv_slider_set_value(music_progress,500,LV_ANIM_OFF);lv_obj_send_event(music_progress,LV_EVENT_VALUE_CHANGED,NULL);
  dock_music_render("正在播放","我的音乐","示例歌曲","示例歌手","示例专辑",1,0,0,68,191,60,"",1,0,1);assert(lv_slider_get_value(music_progress)==500);lv_obj_send_event(music_progress,LV_EVENT_RELEASED,NULL);assert(dock_poll(&event)==1&&!strcmp(event.action,"play.seek")&&!strcmp(event.text,"95"));
  lv_obj_send_event(music_volume_button,LV_EVENT_CLICKED,NULL);assert(!lv_obj_has_flag(music_volume_overlay,LV_OBJ_FLAG_HIDDEN));screenshot("build/native/music-volume.ppm");lv_slider_set_value(music_volume,0,LV_ANIM_OFF);lv_obj_send_event(music_volume,LV_EVENT_RELEASED,NULL);assert(dock_poll(&event)==1&&!strcmp(event.action,"volume.set"));assert(dock_music_back()==1);
  dock_music_playlist("12","我喜欢的音乐",24,1);dock_music_playlist("13","午后与远方",120,0);lv_obj_send_event(music_library_button,LV_EVENT_CLICKED,NULL);
  dock_theme_appearance(0x2563eb,1);lv_obj_t *row=lv_obj_get_child(music_playlists,1),*name=lv_obj_get_child(row,0);assert(lv_color_eq(lv_obj_get_style_text_color(name,0),dock_color(0x0f172a)));assert(lv_obj_get_style_text_align(name,0)==LV_TEXT_ALIGN_LEFT);lv_obj_update_layout(music_screen);lv_area_t row_area,name_area;lv_obj_get_coords(row,&row_area);lv_obj_get_coords(name,&name_area);assert(name_area.x1-row_area.x1==16);screenshot("build/native/music-playlists-light.ppm");dock_theme_appearance(0x0f766e,0);screenshot("build/native/music-playlists-dark.ppm");
  lv_obj_send_event(row,LV_EVENT_CLICKED,NULL);assert(dock_poll(&event)==1&&!strcmp(event.action,"playlist.select")&&!strcmp(event.text,"13"));
  for(int i=0;i<20;i++)dock_music_track(i,"示例歌曲标题","示例歌手 / 专辑",i==0);dock_music_tracks_page(0,6,120);screenshot("build/native/music-tracks.ppm");lv_obj_send_event(lv_obj_get_child(music_tracks,3),LV_EVENT_CLICKED,NULL);assert(dock_poll(&event)==1&&!strcmp(event.action,"track.play"));
  dock_music_cover(NULL,0,0);assert(!music_cover_pixels);assert(!lv_obj_has_flag(music_fallback,LV_OBJ_FLAG_HIDDEN));dock_show_catalog();dock_music_unmount();
 }
 assert(dock_settings_mount()==0);dock_settings_volume(60,"");lv_obj_send_event(system_tab,LV_EVENT_CLICKED,NULL);screenshot("build/native/system-settings.ppm");dock_show_catalog();dock_settings_unmount();dock_close();puts("Music cover, lyrics, seek, volume, playlists and login UI passed");return 0;
}
