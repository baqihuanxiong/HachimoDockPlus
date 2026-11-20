/* Exercise the actual LVGL input path with a pipe in place of evdev. */
#include <assert.h>
#include "../internal/ui/lvgl/bridge.c"
#include "../internal/ui/lvgl/statusbar.c"
#include "../internal/ui/lvgl/theme.c"
#include "../internal/ui/lvgl/catalog.c"
#include "../internal/ui/lvgl/screensaver.c"
#include "../internal/ui/lvgl/page_picoclaw.c"
#include "../internal/ui/lvgl/page_settings.c"

static void send_input(int fd,int type,int code,int value) {
    struct input_event e={0};e.type=type;e.code=code;e.value=value;
    assert(write(fd,&e,sizeof(e))==(ssize_t)sizeof(e));
}

static void pointer_point(int fd,int x,int y){
 send_input(fd,EV_ABS,ABS_MT_POSITION_X,479-y);send_input(fd,EV_ABS,ABS_MT_POSITION_Y,x);send_input(fd,EV_SYN,SYN_REPORT,0);lv_indev_read(touch_device);
}
static void settle(void){for(int i=0;i<70;i++){usleep(10000);dock_tick();}}
static void swipe(int fd,int x,int y,int target_x,int target_y){
 send_input(fd,EV_ABS,ABS_MT_SLOT,7);send_input(fd,EV_ABS,ABS_MT_TRACKING_ID,19);pointer_point(fd,x,y);
 for(int i=1;i<=12;i++){usleep(15000);pointer_point(fd,x+(target_x-x)*i/12,y+(target_y-y)*i/12);dock_tick();}
 send_input(fd,EV_ABS,ABS_MT_TRACKING_ID,-1);send_input(fd,EV_SYN,SYN_REPORT,0);lv_indev_read(touch_device);settle();
}

/* A populated reply must occupy layout space and draw glyphs inside the chat
   viewport, including after replacing an empty streaming placeholder. */
static void assert_reply_visible(void){
    lv_refr_now(display);
    lv_obj_t *row=lv_obj_get_child(history_box,-1);
    lv_obj_t *reply=lv_obj_get_child(row,0);
    assert(lv_obj_get_height(reply)>=lv_font_get_line_height(dock_font_body));
    assert(lv_obj_get_height(row)>=lv_obj_get_height(reply));
    lv_color_t background=lv_obj_get_style_bg_color(history_box,0);
    for(unsigned i=0;i<lv_spangroup_get_span_count(reply);i++){
        lv_style_value_t color;
        assert(lv_style_get_prop(lv_span_get_style(lv_spangroup_get_child(reply,i)),LV_STYLE_TEXT_COLOR,&color)==LV_STYLE_RES_FOUND);
        assert(abs((int)lv_color_brightness(color.color)-(int)lv_color_brightness(background))>=80);
    }
    lv_area_t text_area,viewport;lv_obj_get_coords(reply,&text_area);lv_obj_get_coords(history_box,&viewport);
    assert(text_area.y2>=viewport.y1&&text_area.y1<=viewport.y2);
    lv_color_t ink=dock_color(dock_theme_resolve(0xe2e8f0));
    const unsigned char *pixels=dock_pixels();int glyph_pixels=0;
    for(int y=LV_MAX(text_area.y1,viewport.y1);y<=LV_MIN(text_area.y2,viewport.y2);y++)
        for(int x=LV_MAX(text_area.x1,viewport.x1);x<=LV_MIN(text_area.x2,viewport.x2);x++){
            const unsigned char *p=pixels+((size_t)y*640+x)*4;
            if(p[2]==ink.red&&p[1]==ink.green&&p[0]==ink.blue)glyph_pixels++;
        }
    assert(glyph_pixels>0);
}

static void assert_streamed_markdown(const char *source,const char *expected){
    char history[2048],rendered[2048]="";
    snprintf(history,sizeof(history),"a%zu:%s",strlen(source),source);
    dock_picoclaw_render("正在回复…","","",history,"",1,0,1,1,0);
    lv_obj_t *reply=lv_obj_get_child(lv_obj_get_child(history_box,-1),0);
    for(unsigned i=0;i<lv_spangroup_get_span_count(reply);i++)
        strcat(rendered,lv_span_get_text(lv_spangroup_get_child(reply,i)));
    assert(!strcmp(rendered,expected));
    lv_style_value_t font;
    assert(lv_style_get_prop(lv_span_get_style(lv_spangroup_get_child(reply,0)),LV_STYLE_TEXT_FONT,&font)==LV_STYLE_RES_FOUND);
    assert(font.ptr==dock_font_title);
    assert(lv_obj_has_state(send_button,LV_STATE_DISABLED)); /* Still streaming. */
    lv_refr_now(display);
}

int main(int argc,char **argv) {
    assert(argc==2);
    assert(dock_init(NULL,NULL,argv[1])==0);
    assert(dock_catalog_activate()==0); /* empty catalog */
    dock_catalog_add("first","First","first test app","1");
    dock_catalog_add("second","Second","second test app",LV_SYMBOL_SETTINGS);
    for(int i=0;i<7;i++){char id[32];snprintf(id,sizeof(id),"extra-%d",i);dock_catalog_add(id,id,"extra test app","E");}
    assert(lv_obj_get_child_count(catalog_list)==3);
    assert(lv_obj_get_child_count(lv_obj_get_child(catalog_list,0))==4);
    assert(lv_obj_get_child_count(lv_obj_get_child(catalog_list,2))==1);
    assert(lv_obj_get_child_count(catalog_dots)==3);
    assert(lv_obj_get_scrollbar_mode(catalog_list)==LV_SCROLLBAR_MODE_OFF);
    assert(lv_obj_get_scroll_dir(catalog_list)==LV_DIR_HOR);
    lv_obj_t *first=lv_obj_get_child(lv_obj_get_child(catalog_list,0),0);
    assert(lv_obj_get_child_count(first)==4); /* badge, running label, title, description; no button */
    unsigned char icon_pixels[]={0,0,255,128,0,255,0,255};
    assert(dock_catalog_image("missing",icon_pixels,2,1)==-2);
    assert(dock_catalog_image("first",NULL,2,1)==-1);
    for(int n=0;n<6;n++){
        assert(dock_catalog_image("first",icon_pixels,2,1)==0);
        lv_refr_now(display);
    }
    catalog_entry *image_entry=catalog_entries;while(image_entry&&strcmp(image_entry->id,"first"))image_entry=image_entry->next;
    assert(image_entry&&image_entry->image_pixels!=icon_pixels&&image_entry->image_data.data_size==8);
    assert(image_entry->image_pixels[2]==255&&image_entry->image_pixels[3]==128);
    assert(!lv_obj_has_flag(image_entry->image,LV_OBJ_FLAG_CLICKABLE));
    dock_catalog_image_fill("first");lv_obj_update_layout(catalog);assert(lv_obj_get_width(image_entry->image)==52);assert(lv_obj_get_style_clip_corner(lv_obj_get_parent(image_entry->image),0));
    dock_catalog_running("first",1);assert(!lv_obj_has_flag(lv_obj_get_child(first,1),LV_OBJ_FLAG_HIDDEN));
    dock_catalog_running("first",0);assert(lv_obj_has_flag(lv_obj_get_child(first,1),LV_OBJ_FLAG_HIDDEN));
    dock_theme_set(0x7c3aed);
    assert(lv_color_eq(lv_obj_get_style_bg_color(lv_obj_get_child(catalog_dots,0),0),dock_color(0x7c3aed)));
    dock_theme_set(0x0f766e);
    /* Verify the rendered gear itself is centered, including font baseline
       and transform effects, rather than only checking the label rectangle. */
    lv_refr_now(display);
    lv_obj_t *gear_badge=lv_obj_get_child(lv_obj_get_child(lv_obj_get_child(catalog_list,0),1),0);
    lv_obj_t *gear_symbol=lv_obj_get_child(gear_badge,0);
    lv_area_t gear_area;lv_obj_get_coords(gear_badge,&gear_area);
    lv_color_t ink=lv_obj_get_style_text_color(gear_symbol,0);
    int gear_top=480,gear_bottom=-1;const unsigned char *rendered=dock_pixels();
    for(int y=gear_area.y1;y<=gear_area.y2;y++)for(int x=gear_area.x1;x<=gear_area.x2;x++){
      const unsigned char *p=rendered+((size_t)y*640+x)*4;
      if(p[2]==ink.red&&p[1]==ink.green&&p[0]==ink.blue){if(y<gear_top)gear_top=y;if(y>gear_bottom)gear_bottom=y;}
    }
    assert(gear_bottom>=gear_top);
    assert(abs((gear_top+gear_bottom)-(gear_area.y1+gear_area.y2))<=2);
    dock_catalog_header("11-14 20:30",80,1,4,2);
    lv_obj_update_layout(catalog);
    assert(!strcmp(lv_label_get_text(header_clock),"11-14 20:30"));
    assert(header_battery_level==80&&header_wifi_level==4&&header_bluetooth_kind==2);
    assert(!lv_obj_has_flag(header_bluetooth,LV_OBJ_FLAG_HIDDEN));
    lv_area_t time_bounds,battery_bounds,wifi_bounds,bt_bounds;
    lv_obj_get_coords(header_clock,&time_bounds);lv_obj_get_coords(header_battery,&battery_bounds);
    lv_obj_get_coords(header_wifi,&wifi_bounds);lv_obj_get_coords(header_bluetooth,&bt_bounds);
    assert(time_bounds.x2<bt_bounds.x1&&bt_bounds.x2<wifi_bounds.x1&&wifi_bounds.x2<battery_bounds.x1&&battery_bounds.x2<640);
    assert(battery_bounds.y1==wifi_bounds.y1&&wifi_bounds.y1==bt_bounds.y1);
    assert(battery_color(20)==0x4ade80&&battery_color(10)==0xfb923c&&battery_color(9)==0xf87171);
    for(int kind=1;kind<=10;kind++){
        dock_catalog_header("11-14 20:31",kind*10,0,kind%5,kind);
        lv_refr_now(display);
    }
    dock_catalog_header("11-14 20:31",9,0,0,0);
    lv_refr_now(display);
    dock_catalog_header("等待对时",-1,0,0,0);
    assert(lv_obj_has_flag(header_bluetooth,LV_OBJ_FLAG_HIDDEN));
    assert(header_wifi_level==0&&header_battery_level==-1);
    const uint32_t icons[]={0xf55a,0xf8a2,0xf11c,0xf00c,0xf00d,0xf053,0xf054,0xf1eb,0xf293,0xf244,0xf0e7,0xf013};
    for(unsigned i=0;i<sizeof(icons)/sizeof(icons[0]);i++) {
        lv_font_glyph_dsc_t glyph={0};
        assert(lv_font_get_glyph_dsc(dock_font_body,&glyph,icons[i],0));
        assert(!glyph.is_placeholder&&glyph.resolved_font==&lv_font_montserrat_22);
        lv_font_glyph_release_draw_data(&glyph);
    }
    const uint32_t emoji[]={0x1f99e,0x1f604,0x1f44b,0x2764};
    const lv_font_t *chat_fonts[]={dock_font_body,dock_font_title,dock_font_small};
    for(unsigned f=0;f<sizeof(chat_fonts)/sizeof(chat_fonts[0]);f++)
        for(unsigned i=0;i<sizeof(emoji)/sizeof(emoji[0]);i++){
            lv_font_glyph_dsc_t glyph={0};
            assert(lv_font_get_glyph_dsc(chat_fonts[f],&glyph,emoji[i],0));
            assert(!glyph.is_placeholder&&glyph.box_w>0&&glyph.box_h>0);
            assert(lv_font_get_glyph_bitmap(&glyph,NULL));
            lv_font_glyph_release_draw_data(&glyph);
        }
    lv_display_set_resolution(display,480,640);
    lv_display_set_rotation(display,LV_DISPLAY_ROTATION_270);
    lv_obj_update_layout(catalog);
    assert(lv_display_get_horizontal_resolution(display)==640);
    assert(lv_display_get_vertical_resolution(display)==480);

    int fds[2];assert(pipe(fds)==0);
    touch_fd=fds[0];fcntl(touch_fd,F_SETFL,O_NONBLOCK);
    touch_device=lv_indev_create();
    lv_indev_set_type(touch_device,LV_INDEV_TYPE_POINTER);
    lv_indev_set_read_cb(touch_device,touch_read);
    lv_indev_set_display(touch_device,display);

    /* A nonzero tracking ID in slot 7, without BTN_TOUCH, must click the
       catalog button after native (353,64) -> landscape (64,126). */
    send_input(fds[1],EV_ABS,ABS_MT_SLOT,7);
    send_input(fds[1],EV_ABS,ABS_MT_TRACKING_ID,19);
    send_input(fds[1],EV_ABS,ABS_MT_POSITION_X,353);
    send_input(fds[1],EV_ABS,ABS_MT_POSITION_Y,64);
    send_input(fds[1],EV_SYN,SYN_REPORT,0);
    lv_indev_read(touch_device);
    lv_point_t point;lv_indev_get_point(touch_device,&point);
    assert(point.x==64&&point.y==126);
    send_input(fds[1],EV_ABS,ABS_MT_TRACKING_ID,-1);
    send_input(fds[1],EV_SYN,SYN_REPORT,0);
    lv_indev_read(touch_device);
    dock_event event;assert(dock_poll(&event)==1&&!strcmp(event.action,"shell.open")&&!strcmp(event.text,"first"));
    assert(dock_poll(&event)==0);
    lv_obj_t *second=lv_obj_get_child(lv_obj_get_child(catalog_list,0),1);
    lv_obj_send_event(second,LV_EVENT_CLICKED,NULL);
    assert(dock_poll(&event)==1&&!strcmp(event.text,"second")&&!strcmp(event.action,"shell.open"));
    swipe(fds[1],540,350,100,350);
    assert(catalog_page_index==1&&lv_obj_get_scroll_x(catalog_list)==600);
    assert(dock_poll(&event)==0); /* horizontal drag must not open a card */
    swipe(fds[1],540,350,540,150);
    assert(catalog_page_index==1&&lv_obj_get_scroll_y(catalog_list)==0);
    assert(dock_poll(&event)==0); /* vertical drag is ignored */
    lv_obj_send_event(lv_obj_get_child(catalog_dots,2),LV_EVENT_CLICKED,NULL);settle();
    assert(catalog_page_index==2);
    lv_obj_scroll_to_x(catalog_list,600,LV_ANIM_OFF);assert(catalog_page_index==1);
    assert(lv_color_eq(lv_obj_get_style_bg_color(lv_obj_get_child(catalog_dots,1),0),dock_color(0x0f766e)));
    lv_obj_scroll_to_x(catalog_list,0,LV_ANIM_OFF);
    assert(dock_catalog_move(-1,0)==0); /* outer edge keeps the first card */
    assert(dock_catalog_move(1,0)==1);
    assert(dock_catalog_move(0,1)==3);
    assert(dock_catalog_move(1,0)==6&&catalog_page_index==1);
    assert(dock_catalog_move(1,0)==7);
    assert(dock_catalog_move(1,0)==7); /* missing bottom row on last page */
    assert(dock_catalog_move(0,-1)==5);
    assert(dock_catalog_move(1,0)==8&&catalog_page_index==2);
    assert(dock_catalog_activate()==1);
    assert(dock_poll(&event)==1&&!strcmp(event.action,"shell.open")&&!strcmp(event.text,"extra-6"));
    assert(dock_catalog_move(1,0)==8); /* outer edge */
    lv_obj_scroll_to_x(catalog_list,0,LV_ANIM_OFF);
    assert(catalog_selection==0); /* touch page change resets selection */
    assert(dock_catalog_move(0,1)==2);
    dock_screensaver_show("clock");dock_screensaver_time("20:30:15","2026-11-14 Sat");
    assert(lv_screen_active()==screensaver&&!strcmp(lv_label_get_text(saver_clock),"20:30:15"));
    assert(lv_obj_get_style_opa(screensaver,0)<LV_OPA_COVER);
    settle();assert(lv_obj_get_style_opa(screensaver,0)==LV_OPA_COVER);
    lv_obj_send_event(screensaver,LV_EVENT_CLICKED,NULL);
    assert(dock_poll(&event)==1&&!strcmp(event.action,"shell.wake"));
    assert(dock_catalog_activate()==0); /* SW2 cannot open a hidden card */
    dock_screensaver_show("blank");assert(lv_obj_has_flag(saver_clock,LV_OBJ_FLAG_HIDDEN));
    dock_show_catalog();
    /* Wake immediately, before a single fade frame, then let animation timers
       run past completion. No stale screen callback may load freed memory. */
    dock_screensaver_close();
    dock_screensaver_show("pet");dock_show_catalog();dock_screensaver_close();
    settle();assert(lv_screen_active()==catalog);
    for(int n=0;n<3;n++) {
        dock_screensaver_show("pet");assert(lv_obj_has_flag(saver_clock,LV_OBJ_FLAG_HIDDEN));
        unsigned char *frame=calloc(640*480,4);assert(frame);frame[(240*640+320)*4+2]=255;
        assert(dock_screensaver_frame(frame,640*480*4)==0);free(frame);
        dock_screensaver_pet("西高地 · 已连接 · 工作中","任务标题","正在执行：等待 Agent 完成当前任务，然后继续下一步。");
        dock_screensaver_time("20:30:15","2026-11-21 Sat");settle();lv_refr_now(display);
        const unsigned char *pet_pixels=dock_pixels();assert(pet_pixels[(240*640+320)*4+2]==255);
        assert(!strcmp(lv_label_get_text(saver_pet_title),"任务标题"));
        assert(!strcmp(lv_label_get_text(saver_pet_datetime),"20:30"));
        assert(!lv_obj_has_flag(saver_pet_datetime,LV_OBJ_FLAG_HIDDEN));
        assert(lv_obj_get_style_text_font(saver_pet_status,0)==dock_font_body);
        assert(lv_obj_get_style_text_font(saver_pet_text,0)==dock_font_body);
        assert(lv_obj_get_x(saver_pet_status)+lv_obj_get_width(saver_pet_status)<lv_obj_get_x(saver_pet_datetime));
        assert(lv_obj_get_height(saver_pet_text)>=2*lv_font_get_line_height(dock_font_body));
        /* Use actual pointer input on the image, header, clock and Agent
           bubble: none of these overlays may eat taps or wake the catalog. */
        const int taps[][2]={{80,240},{320,240},{560,240},{80,24},{560,24},{80,414}};
        for(unsigned i=0;i<sizeof(taps)/sizeof(taps[0]);i++){
            send_input(fds[1],EV_ABS,ABS_MT_SLOT,7);send_input(fds[1],EV_ABS,ABS_MT_TRACKING_ID,20);
            pointer_point(fds[1],taps[i][0],taps[i][1]);
            send_input(fds[1],EV_ABS,ABS_MT_TRACKING_ID,-1);send_input(fds[1],EV_SYN,SYN_REPORT,0);lv_indev_read(touch_device);
            assert(dock_poll(&event)==1&&!strcmp(event.action,"shell.pet.touch")&&atoi(event.text)==taps[i][0]);
            assert(dock_poll(&event)==0&&lv_screen_active()==screensaver);
        }
        dock_screensaver_time("等待对时","");assert(!strcmp(lv_label_get_text(saver_pet_datetime),"等待对时"));
        dock_screensaver_show("clock");assert(lv_obj_has_flag(saver_pet_datetime,LV_OBJ_FLAG_HIDDEN));
        dock_show_catalog();dock_screensaver_close();assert(!saver_pet_pixels);
    }
    lv_obj_remove_state(first,LV_STATE_FOCUSED);
    for(int i=0;i<3;i++) {
        assert(dock_picoclaw_mount()==0);
        assert(dock_catalog_activate()==0);
        dock_theme_appearance(0x0f766e,1);
        assert(lv_color_eq(lv_obj_get_style_bg_color(chat,0),dock_color(0xf3f5f9)));
        assert(lv_color_eq(lv_obj_get_style_text_color(hint_label,0),dock_color(0x52627a)));
        lv_refr_now(display);
        dock_theme_appearance(0x0f766e,0);
        dock_picoclaw_render("就绪","","按住 SW1 说话","","",0,0,0,1,0);
        assert_reply_visible();
        lv_area_t title_area,status_area,history_area;
        lv_obj_get_coords(lv_obj_get_child(chat,1),&title_area);
        lv_obj_get_coords(status_label,&status_area);lv_obj_get_coords(history_box,&history_area);
        assert(status_area.y1>=title_area.y1&&status_area.y2<=title_area.y2);
        assert(status_area.x1>title_area.x2&&status_area.x2<510);
        assert(history_area.y1==72&&lv_obj_get_height(history_box)==274);
        lv_area_t clear_area,send_area,stop_area;
        lv_obj_get_coords(clear_button,&clear_area);lv_obj_get_coords(send_button,&send_area);lv_obj_get_coords(speech_stop,&stop_area);
        assert(stop_area.x1==clear_area.x1&&stop_area.x2==send_area.x2);
        dock_picoclaw_render("思考中","","","u2:hia0:","",1,0,1,1,0);
        const char *replies[]={"嗨！我是 PicoClaw 🦞 有什么想聊的或者需要帮忙的吗？😄","# 回复\n你好，**欢迎**使用 `PicoClaw`。\n- 第一条建议\n- 第二条建议"};
        for(unsigned n=0;n<sizeof(replies)/sizeof(replies[0]);n++){
            char history[1024];snprintf(history,sizeof(history),"u2:hia%zu:%s",strlen(replies[n]),replies[n]);
            dock_picoclaw_render("回复中","","",history,"",1,0,1,1,0);
            assert_reply_visible();
        }
        assert_streamed_markdown("# 标题\n**正在生成","标题\n正在生成");
        assert_streamed_markdown("# 标题\n**正在生成**\n- 第一条\n`**literal**`\n```go\na()\nb()\n```",
                                 "标题\n正在生成\n• 第一条\n**literal**\na()\nb()");
        dock_picoclaw_render("准备就绪","","","","",1,0,0,1,0);
        assert_reply_visible();
        dock_theme_appearance(0x0f766e,1);
        assert_reply_visible();
        char light_history[1024];snprintf(light_history,sizeof(light_history),"a%zu:%s",strlen(replies[0]),replies[0]);
        dock_picoclaw_render("准备就绪","","",light_history,"",1,0,0,1,0);
        assert_reply_visible();
        dock_theme_appearance(0x0f766e,0);
        assert_reply_visible();
        assert(lv_obj_has_state(speech_switch,LV_STATE_CHECKED));
        assert(lv_obj_has_state(speech_stop,LV_STATE_DISABLED));
        lv_obj_remove_state(speech_switch,LV_STATE_CHECKED);
        lv_obj_send_event(speech_switch,LV_EVENT_VALUE_CHANGED,NULL);
        assert(dock_poll(&event)==1&&!strcmp(event.action,"speech.set")&&!strcmp(event.text,"false"));
        dock_picoclaw_render("正在播放回复…","","",light_history,"",1,0,1,1,1);
        assert(lv_obj_has_state(speech_switch,LV_STATE_CHECKED)&&!lv_obj_has_state(speech_stop,LV_STATE_DISABLED));
        assert(dock_poll(&event)==0); /* Applying state must not emit switch events. */
        lv_obj_send_event(speech_stop,LV_EVENT_CLICKED,NULL);
        assert(dock_poll(&event)==1&&!strcmp(event.action,"speech.stop"));
        dock_picoclaw_render("已停止","","",light_history,"",1,0,0,0,0);
        assert(!lv_obj_has_state(speech_switch,LV_STATE_CHECKED)&&lv_obj_has_state(speech_stop,LV_STATE_DISABLED));
        lv_obj_remove_flag(keyboard,LV_OBJ_FLAG_HIDDEN);
        assert(dock_picoclaw_back()==1&&dock_picoclaw_back()==0);
        lv_textarea_set_text(text_input,"hello");
        lv_obj_send_event(send_button,LV_EVENT_CLICKED,NULL);
        assert(dock_poll(&event)==1&&!strcmp(event.app_id,"picoclaw")&&!strcmp(event.action,"submit")&&!strcmp(event.text,"hello"));
        lv_obj_send_event(lv_obj_get_child(chat,0),LV_EVENT_CLICKED,NULL);
        assert(dock_poll(&event)==1&&!strcmp(event.app_id,"picoclaw")&&!strcmp(event.action,"shell.back"));
        dock_show_catalog();dock_picoclaw_unmount();
    }
    for(int i=0;i<3;i++) {
        assert(dock_settings_mount()==0);
        dock_settings_theme_add("teal","青绿",0x0f766e);
        dock_settings_theme_add("violet","紫色",0x7c3aed);
        dock_settings_theme_add("blue","蓝色",0x2563eb);
        dock_settings_theme_add("amber","琥珀",0xd97706);
        dock_settings_theme_add("rose","玫红",0xdb2777);
        dock_settings_theme_add("cyan","青色",0x0891b2);
        dock_settings_theme_add("orange","橙色",0xea580c);
        dock_settings_theme_add("green","绿色",0x16a34a);
        dock_settings_theme_add("red","红色",0xdc2626);
        assert(theme_choice_count==9);
        dock_settings_theme("teal");
        assert(lv_color_eq(lv_obj_get_style_bg_color(theme_choices[0].button,0),dock_color(0x0f766e)));
        dock_settings_render("就绪","Home 192.168.1.2","蓝牙已开启",1,1,0);
        dock_settings_system(1,0,1,"dockconfig","请先设置共享账号","\\\\192.168.1.2\\HachimoConfig");
        lv_obj_send_event(system_tab,LV_EVENT_CLICKED,NULL);
        assert(!lv_obj_has_flag(system_panel,LV_OBJ_FLAG_HIDDEN));
        assert(lv_obj_get_parent(audio_slider)==system_panel);
        assert(lv_obj_get_parent(pet_status)==system_panel&&lv_obj_get_parent(pet_forget)==system_panel);
        dock_settings_pet("等待 Desktop 配对",0);
        assert(lv_obj_has_state(pet_forget,LV_STATE_DISABLED));
        dock_settings_pet("已配对 · 等待 Desktop",1);
        assert(!lv_obj_has_state(pet_forget,LV_STATE_DISABLED));
        lv_obj_update_layout(system_panel);
        lv_obj_scroll_to_view(pet_status,LV_ANIM_OFF);
        lv_obj_update_layout(system_panel);
        lv_area_t pair_bounds,pair_status_bounds;lv_obj_get_coords(pet_forget,&pair_bounds);lv_obj_get_coords(pet_status,&pair_status_bounds);
        assert(pair_bounds.y1>=106&&pair_bounds.y2<pair_status_bounds.y1&&pair_status_bounds.y2<480);
        assert(pair_bounds.x1>=438&&pair_bounds.x2<624);
        lv_obj_send_event(pet_forget,LV_EVENT_CLICKED,NULL);
        assert(dock_poll(&event)==1&&!strcmp(event.action,"pet.forget"));
        dock_settings_render("正在处理…","Home","蓝牙已开启",1,1,1);
        dock_settings_pet("已配对 · 等待 Desktop",1);
        assert(lv_obj_has_state(pet_forget,LV_STATE_DISABLED));
        dock_settings_render("就绪","Home","蓝牙已开启",1,1,0);
        dock_settings_pet("等待 Desktop 配对",0);
        assert(abs(lv_obj_get_y(smb_switch)-lv_obj_get_y(smb_account_button))<=12);
        dock_settings_audio("speaker","","","");
        assert(lv_obj_has_state(audio_bluetooth,LV_STATE_DISABLED));
        dock_settings_audio("bluetooth","11:22:33:44:55:66","测试耳机 (11:22:33:44:55:66)","11:22:33:44:55:66");
        assert(audio_device_count==1&&!lv_obj_has_state(audio_bluetooth,LV_STATE_DISABLED));
        lv_obj_send_event(audio_bluetooth,LV_EVENT_CLICKED,NULL);
        assert(dock_poll(&event)==1&&!strcmp(event.action,"audio.bluetooth")&&!strcmp(event.text,"11:22:33:44:55:66"));
        lv_obj_send_event(audio_spdif,LV_EVENT_CLICKED,NULL);
        assert(dock_poll(&event)==1&&!strcmp(event.action,"audio.output")&&!strcmp(event.text,"spdif"));
        dock_settings_audio("spdif","11:22:33:44:55:66","测试耳机 (11:22:33:44:55:66)","11:22:33:44:55:66");
        assert(dock_poll(&event)==0); /* Rendering cannot change preferences. */
        lv_obj_update_layout(system_panel);
        lv_obj_scroll_to_view(system_restart_button,LV_ANIM_OFF);
        lv_obj_update_layout(system_panel);
        lv_area_t restart_bounds;lv_obj_get_coords(system_restart_button,&restart_bounds);
        assert(restart_bounds.y1>=106&&restart_bounds.y2<480);
        lv_obj_send_event(smb_account_button,LV_EVENT_CLICKED,NULL);
        assert(system_dialog&&lv_textarea_get_password_mode(smb_password_input));
        lv_textarea_set_text(smb_username_input,"docktest");
        lv_textarea_set_text(smb_password_input,"long\"password\\123");
        dock_settings_system(1,0,1,"different","服务状态刷新","test-address");
        assert(!strcmp(lv_textarea_get_text(smb_username_input),"docktest"));
        assert(!strcmp(lv_textarea_get_text(smb_password_input),"long\"password\\123"));
        lv_obj_update_layout(system_dialog);
        lv_area_t system_keyboard_bounds;lv_obj_get_coords(system_keyboard,&system_keyboard_bounds);
        assert(system_keyboard_bounds.y1>=262&&system_keyboard_bounds.y2<480);
        lv_obj_send_event(lv_obj_get_child(system_dialog,3),LV_EVENT_CLICKED,NULL);
        assert(!system_dialog&&!smb_password_input);
        assert(dock_poll(&event)==1&&!strcmp(event.action,"smb.account"));
        assert(strstr(event.text,"docktest")&&strstr(event.text,"long\\\"password\\\\123"));
        lv_obj_add_state(smb_switch,LV_STATE_CHECKED);lv_obj_send_event(smb_switch,LV_EVENT_VALUE_CHANGED,NULL);
        assert(dock_poll(&event)==1&&!strcmp(event.action,"smb.set")&&!strcmp(event.text,"true"));
        dock_settings_system(1,0,1,"docktest","共享已关闭","test-address");
        assert(dock_poll(&event)==0); /* Rendering switch state cannot trigger a write. */
        lv_obj_send_event(system_restart_button,LV_EVENT_CLICKED,NULL);
        assert(system_dialog&&dock_settings_back()==1&&!system_dialog);
        lv_obj_send_event(system_restart_button,LV_EVENT_CLICKED,NULL);
        lv_obj_send_event(lv_obj_get_child(system_dialog,2),LV_EVENT_CLICKED,NULL);
        assert(!system_dialog&&dock_poll(&event)==1&&!strcmp(event.action,"system.restart"));
        lv_obj_send_event(appearance_tab,LV_EVENT_CLICKED,NULL);
        assert(!lv_obj_has_flag(appearance_panel,LV_OBJ_FLAG_HIDDEN));
        lv_obj_update_layout(appearance_panel);
        lv_area_t last_color;lv_obj_get_coords(theme_choices[8].button,&last_color);
        assert(last_color.y1>=310&&last_color.y2<362&&last_color.x2<640);
        dock_theme_appearance(0x0f766e,1);dock_settings_mode("light");
        lv_refr_now(display);
        assert(lv_color_eq(lv_obj_get_style_bg_color(settings_screen,0),dock_color(0xf3f5f9)));
        assert(lv_color_eq(lv_obj_get_style_text_color(theme_selected,0),dock_color(0x1e293b)));
        assert(lv_color_eq(lv_obj_get_style_bg_color(first,0),dock_color(0xffffff)));
        assert(dock_theme_resolve(0xe2e8f0)==0x1e293b&&dock_theme_resolve(0x121d2c)==0xe5eaf2);
        assert(lv_color_eq(lv_obj_get_style_bg_color(theme_choices[0].button,0),dock_color(0x0f766e)));
        lv_obj_send_event(mode_dark_button,LV_EVENT_CLICKED,NULL);
        assert(dock_poll(&event)==1&&!strcmp(event.action,"mode.set")&&!strcmp(event.text,"dark"));
        dock_theme_appearance(0x0f766e,0);dock_settings_mode("dark");
        assert(lv_color_eq(lv_obj_get_style_bg_color(settings_screen,0),dock_color(0x151515)));
        lv_obj_send_event(theme_choices[1].button,LV_EVENT_CLICKED,NULL);
        assert(dock_poll(&event)==1&&!strcmp(event.action,"theme.set")&&!strcmp(event.text,"violet"));
        dock_theme_set(0x7c3aed);dock_settings_theme("violet");
        assert(lv_obj_has_state(theme_choices[1].button,LV_STATE_CHECKED));
        assert(lv_color_eq(lv_obj_get_style_bg_color(wifi_scan,0),dock_color(0x7c3aed)));
        assert(lv_color_eq(lv_obj_get_style_bg_color(theme_choices[0].button,0),dock_color(0x0f766e)));
        assert(lv_obj_get_parent(saver_blank_button)==appearance_panel);
        lv_area_t saver_bounds;lv_obj_get_coords(saver_blank_button,&saver_bounds);
        assert(last_color.y2<saver_bounds.y1&&saver_bounds.y2<438);
        lv_obj_send_event(saver_blank_button,LV_EVENT_CLICKED,NULL);
        assert(dock_poll(&event)==1&&!strcmp(event.action,"screensaver.set")&&!strcmp(event.text,"blank"));
        dock_settings_standby(300);assert(lv_dropdown_get_selected(standby_dropdown)==4&&dock_poll(&event)==0);
        lv_obj_scroll_to_view(standby_dropdown,LV_ANIM_OFF);lv_obj_update_layout(appearance_panel);
        lv_area_t standby_bounds;lv_obj_get_coords(standby_dropdown,&standby_bounds);
        assert(standby_bounds.y1>=106&&standby_bounds.y2<480);
        lv_dropdown_set_selected(standby_dropdown,1);lv_obj_send_event(standby_dropdown,LV_EVENT_VALUE_CHANGED,NULL);
        assert(dock_poll(&event)==1&&!strcmp(event.action,"standby.set")&&!strcmp(event.text,"30"));
        dock_screensaver_show("pet");settle();dock_screensaver_restore();
        assert(lv_screen_active()==settings_screen&&!screensaver);
        /* A wake tap on a tab must not also switch tabs, including release. */
        uint64_t activity_before=dock_input_activity();dock_input_sleep(1);
        send_input(fds[1],EV_ABS,ABS_MT_SLOT,7);send_input(fds[1],EV_ABS,ABS_MT_TRACKING_ID,21);pointer_point(fds[1],570,34);
        assert(dock_poll(&event)==1&&!strcmp(event.action,"shell.wake"));
        assert(dock_input_activity()>activity_before);
        dock_input_sleep(0);
        send_input(fds[1],EV_ABS,ABS_MT_TRACKING_ID,-1);send_input(fds[1],EV_SYN,SYN_REPORT,0);lv_indev_read(touch_device);settle();
        assert(dock_poll(&event)==0&&!lv_obj_has_flag(appearance_panel,LV_OBJ_FLAG_HIDDEN));
        lv_obj_scroll_to_y(appearance_panel,0,LV_ANIM_OFF);
        open_join("","WPA-PSK");assert(dock_settings_back()==1&&!join_dialog&&dock_settings_back()==0);
        lv_obj_send_event(wifi_tab,LV_EVENT_CLICKED,NULL);
        dock_settings_network("0","Home","已保存",-45,1);
        dock_settings_network("","Cafe \" guest","WPA-PSK",-60,0);
        dock_settings_device("AA:BB:CC:DD:EE:FF","Headphones","耳机",1,0,1,1);
        lv_obj_t *row=lv_obj_get_child(wifi_list,0);
        lv_obj_send_event(lv_obj_get_child(row,2),LV_EVENT_CLICKED,NULL);
        assert(dock_poll(&event)==1&&!strcmp(event.action,"wifi.connect")&&!strcmp(event.text,"0"));
        row=lv_obj_get_child(wifi_list,1);
        lv_obj_send_event(lv_obj_get_child(row,2),LV_EVENT_CLICKED,NULL);
        assert(join_dialog && lv_textarea_get_password_mode(join_password));
        lv_obj_update_layout(join_dialog);
        lv_area_t keyboard_bounds;lv_obj_get_coords(join_keyboard,&keyboard_bounds);
        assert(keyboard_bounds.y1>=250&&keyboard_bounds.y2<480);
        lv_textarea_set_text(join_password,"hello123");
        dock_settings_clear(); /* A refresh must not invalidate dialog fields. */
        lv_obj_send_event(join_keyboard,LV_EVENT_READY,NULL);
        assert(!join_dialog);
        assert(dock_poll(&event)==1&&!strcmp(event.action,"wifi.join"));
        assert(strstr(event.text,"hello123")&&strstr(event.text,"WPA-PSK"));
        dock_settings_device("AA:BB:CC:DD:EE:FF","Headphones","耳机",1,0,1,1);
        row=lv_obj_get_child(bt_list,0);
        lv_obj_send_event(bt_tab,LV_EVENT_CLICKED,NULL);lv_obj_update_layout(bt_list);
        assert(lv_obj_get_x(lv_obj_get_child(row,3))>lv_obj_get_x(lv_obj_get_child(row,2)));
        lv_obj_send_event(lv_obj_get_child(row,3),LV_EVENT_CLICKED,NULL);
        assert(dock_poll(&event)==1&&!strcmp(event.action,"bt.connect")&&!strcmp(event.text,"AA:BB:CC:DD:EE:FF"));
        dock_settings_clear();
        dock_settings_device("11:22:33:44:55:66","Nearby headphones","耳机",0,0,0,1);
        row=lv_obj_get_child(bt_list,0);
        lv_obj_update_layout(bt_list);
        lv_obj_t *pair_button=lv_obj_get_child(row,2);
        assert(lv_obj_get_x(pair_button)==476&&!lv_obj_has_state(pair_button,LV_STATE_DISABLED));
        lv_obj_send_event(pair_button,LV_EVENT_CLICKED,NULL);
        assert(dock_poll(&event)==1&&!strcmp(event.action,"bt.pair")&&!strcmp(event.text,"11:22:33:44:55:66"));
        dock_settings_device("FC:E8:06:30:C7:F0","Saved speaker","音箱",0,0,1,0);
        row=lv_obj_get_child(bt_list,1);
        assert(strstr(lv_label_get_text(lv_obj_get_child(row,1)),"已保存"));
        assert(strstr(lv_label_get_text(lv_obj_get_child(row,1)),"未发现"));
        assert(lv_obj_has_state(lv_obj_get_child(row,3),LV_STATE_DISABLED));
        lv_obj_send_event(lv_obj_get_child(row,2),LV_EVENT_CLICKED,NULL);
        assert(dock_poll(&event)==1&&!strcmp(event.action,"bt.remove")&&!strcmp(event.text,"FC:E8:06:30:C7:F0"));
        dock_show_catalog();dock_settings_unmount();
    }
    close(fds[1]);dock_close();
    puts("LVGL: keyboard glyph fallback, landscape touch mapping and nonzero tracking ID click passed");
    return 0;
}
