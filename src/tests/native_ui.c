/* Exercise the actual LVGL input path with a pipe in place of evdev. */
#include <assert.h>
#include "../internal/ui/lvgl/bridge.c"
#include "../internal/ui/lvgl/statusbar.c"
#include "../internal/ui/lvgl/theme.c"
#include "../internal/ui/lvgl/catalog.c"
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

int main(int argc,char **argv) {
    assert(argc==2);
    assert(dock_init(NULL,NULL,argv[1])==0);
    dock_catalog_add("first","First","first test app","1");
    dock_catalog_add("second","Second","second test app","2");
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
    dock_catalog_running("first",1);assert(!lv_obj_has_flag(lv_obj_get_child(first,1),LV_OBJ_FLAG_HIDDEN));
    dock_catalog_running("first",0);assert(lv_obj_has_flag(lv_obj_get_child(first,1),LV_OBJ_FLAG_HIDDEN));
    dock_theme_set(0x7c3aed);
    assert(lv_color_eq(lv_obj_get_style_bg_color(lv_obj_get_child(catalog_dots,0),0),dock_color(0x7c3aed)));
    dock_theme_set(0x0f766e);
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
    for(int i=0;i<3;i++) {
        assert(dock_picoclaw_mount()==0);
        dock_theme_appearance(0x0f766e,1);
        assert(lv_color_eq(lv_obj_get_style_bg_color(chat,0),dock_color(0xf3f5f9)));
        assert(lv_color_eq(lv_obj_get_style_text_color(history_label,0),dock_color(0x1e293b)));
        lv_refr_now(display);
        dock_theme_appearance(0x0f766e,0);
        dock_picoclaw_render("就绪","","","开始说话",0,"未接入音频设备",0);
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
        lv_obj_send_event(appearance_tab,LV_EVENT_CLICKED,NULL);
        assert(!lv_obj_has_flag(appearance_panel,LV_OBJ_FLAG_HIDDEN));
        lv_obj_update_layout(appearance_panel);
        lv_area_t last_color;lv_obj_get_coords(theme_choices[8].button,&last_color);
        assert(last_color.y1>=362&&last_color.y2<438&&last_color.x2<640);
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
        assert(lv_color_eq(lv_obj_get_style_bg_color(settings_screen,0),dock_color(0x0c1220)));
        lv_obj_send_event(theme_choices[1].button,LV_EVENT_CLICKED,NULL);
        assert(dock_poll(&event)==1&&!strcmp(event.action,"theme.set")&&!strcmp(event.text,"violet"));
        dock_theme_set(0x7c3aed);dock_settings_theme("violet");
        assert(lv_obj_has_state(theme_choices[1].button,LV_STATE_CHECKED));
        assert(lv_color_eq(lv_obj_get_style_bg_color(wifi_scan,0),dock_color(0x7c3aed)));
        assert(lv_color_eq(lv_obj_get_style_bg_color(theme_choices[0].button,0),dock_color(0x0f766e)));
        lv_obj_send_event(wifi_tab,LV_EVENT_CLICKED,NULL);
        dock_settings_network("0","Home","已保存",-45,1);
        dock_settings_network("","Cafe \" guest","WPA-PSK",-60,0);
        dock_settings_device("AA:BB:CC:DD:EE:FF","Headphones",1,0);
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
        dock_settings_device("AA:BB:CC:DD:EE:FF","Headphones",1,0);
        row=lv_obj_get_child(bt_list,0);
        lv_obj_send_event(lv_obj_get_child(row,2),LV_EVENT_CLICKED,NULL);
        assert(dock_poll(&event)==1&&!strcmp(event.action,"bt.connect")&&!strcmp(event.text,"AA:BB:CC:DD:EE:FF"));
        dock_show_catalog();dock_settings_unmount();
    }
    close(fds[1]);dock_close();
    puts("LVGL: keyboard glyph fallback, landscape touch mapping and nonzero tracking ID click passed");
    return 0;
}
