/* Real LVGL regression for retained, collapsible public process commentary. */
#include <assert.h>
#include "../internal/ui/lvgl/bridge.c"
#include "../internal/ui/lvgl/statusbar.c"
#include "../internal/ui/lvgl/theme.c"
#include "../internal/ui/lvgl/catalog.c"
#include "../internal/ui/lvgl/screensaver.c"
#include "../internal/ui/lvgl/page_picoclaw.c"
#include "src/widgets/span/lv_span_private.h"

static void process_history(char *out,size_t cap,char phase,const char *process,const char *answer){
    snprintf(out,cap,"u3:ask%c%zu:%sa%zu:%s",phase,strlen(process),process,strlen(answer),answer);
}
static void process_pixels(const char *path){
    FILE *file=fopen(path,"wb");assert(file);
    assert(fwrite(dock_pixels(),1,640*480*4,file)==640*480*4);fclose(file);
}
int main(int argc,char **argv){
    assert(argc==2);assert(dock_init(NULL,NULL,argv[1])==0);
    assert(dock_picoclaw_mount()==0);
    char wire[4096];
    const char *steps="阶段一：读取本地技能\n查询已连接的设备\n\n阶段二：检查设备状态\n取得结果后整理回复。";
    process_history(wire,sizeof(wire),'p',steps,"正在整理结果…");
    dock_picoclaw_render("正在处理…","","",wire,"",0,0,1,1,0);
    assert(lv_obj_get_child_count(history_box)==3);
    pico_process_view *view=lv_obj_get_user_data(lv_obj_get_child(history_box,1));
    assert(view&&view->expanded&&view->body);
    lv_obj_update_layout(history_box);
    lv_refr_now(display);
    assert(((lv_spangroup_t *)view->body)->line_count>=5);
    int expanded_height=lv_obj_get_height(view->row);
    lv_obj_send_event(view->header,LV_EVENT_CLICKED,NULL);
    assert(!view->expanded&&lv_obj_has_flag(view->body,LV_OBJ_FLAG_HIDDEN));
    assert(lv_obj_get_height(view->row)<expanded_height);
    /* Updates during execution respect a user's manual collapse. */
    process_history(wire,sizeof(wire),'p',"阶段一\n阶段二\n阶段三", "结果生成中");
    dock_picoclaw_render("回复中","","",wire,"",0,0,1,1,0);
    view=lv_obj_get_user_data(lv_obj_get_child(history_box,1));
    assert(view&&!view->expanded&&!view->body);
    lv_obj_send_event(view->header,LV_EVENT_CLICKED,NULL);assert(view->expanded);
    /* Completion retains the process but automatically folds it. */
    process_history(wire,sizeof(wire),'P',steps,"设备查询已完成。\n这是最终答复，处理过程可点击展开查看。");
    dock_picoclaw_render("准备就绪","","",wire,"",0,0,0,1,0);
    lv_obj_t *saved_row=lv_obj_get_child(history_box,1);
    view=lv_obj_get_user_data(saved_row);
    assert(view&&!view->expanded&&!view->body&&!strcmp(view->text,steps));
    lv_obj_scroll_to_y(history_box,LV_COORD_MAX,LV_ANIM_OFF);
    for(int i=0;i<3;i++){lv_obj_invalidate(history_box);lv_refr_now(display);}
    process_pixels("build/native/picoclaw-process-collapsed.rgba");
    lv_obj_send_event(view->header,LV_EVENT_CLICKED,NULL);
    assert(view->expanded&&view->body);
    lv_obj_scroll_to_y(history_box,0,LV_ANIM_OFF);
    for(int i=0;i<3;i++){lv_obj_invalidate(history_box);lv_refr_now(display);}
    process_pixels("build/native/picoclaw-process-expanded.rgba");
    for(int i=0;i<3;i++)dock_picoclaw_render("准备就绪","","",wire,"",0,0,0,1,0);
    assert(lv_obj_get_child(history_box,1)==saved_row&&view->expanded);
    /* A later message and a theme change do not reset the saved expansion. */
    strncat(wire,"u4:nexta5:hello",sizeof(wire)-strlen(wire)-1);
    dock_picoclaw_render("准备就绪","","",wire,"",0,0,0,1,0);
    assert(lv_obj_get_child(history_box,1)==saved_row&&view->expanded);
    dock_theme_appearance(0x0f766e,1);lv_refr_now(display);
    dock_theme_appearance(0x0f766e,0);lv_refr_now(display);
    dock_show_catalog();dock_picoclaw_unmount();dock_close();
    puts("PicoClaw process folding, retained text, newlines and follow-up rendering passed");
    return 0;
}
