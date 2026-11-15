/* Scroll the real rich-text widget; usable as a native test or board benchmark. */
#include <assert.h>
#include "../internal/ui/lvgl/bridge.c"
#include "../internal/ui/lvgl/statusbar.c"
#include "../internal/ui/lvgl/theme.c"
#include "../internal/ui/lvgl/catalog.c"
#include "../internal/ui/lvgl/screensaver.c"
#include "../internal/ui/lvgl/page_picoclaw.c"
#include "src/widgets/span/lv_span_private.h"

static unsigned long glyph_calls;
static bool (*original_glyph)(const lv_font_t *,lv_font_glyph_dsc_t *,uint32_t,uint32_t);
static bool count_glyph(const lv_font_t *font,lv_font_glyph_dsc_t *dsc,uint32_t ch,uint32_t next){
    glyph_calls++;
    return original_glyph(font,dsc,ch,next);
}
static double now_ms(void){struct timespec ts;clock_gettime(CLOCK_MONOTONIC,&ts);return ts.tv_sec*1000.0+ts.tv_nsec/1e6;}
static uint32_t frozen_ms;
static uint32_t frozen_tick(void){return frozen_ms;}
static int compare_ms(const void *a,const void *b){double x=*(const double *)a,y=*(const double *)b;return (x>y)-(x<y);}
static void force_original(lv_event_t *e){((lv_spangroup_t *)lv_event_get_target(e))->line_count=0;}
static void assert_same_pixels(int y){
    lv_obj_scroll_to_y(history_box,y,LV_ANIM_OFF);
    lv_obj_t *reply=lv_obj_get_child(lv_obj_get_child(history_box,0),0);
    /* Settle deferred layout/style invalidations after resizing or retheming. */
    for(int i=0;i<3;i++){lv_obj_invalidate(history_box);lv_refr_now(display);}
    assert(((lv_spangroup_t *)reply)->line_count>0);
    size_t bytes=640*480*4;
    unsigned char *expected=malloc(bytes);assert(expected);
    memcpy(expected,dock_pixels(),bytes);
    lv_obj_add_event_cb(reply,force_original,LV_EVENT_DRAW_MAIN_BEGIN,NULL);
    for(int i=0;i<3;i++){lv_obj_invalidate(history_box);lv_refr_now(display);}
    assert(!memcmp(expected,dock_pixels(),bytes));
    lv_obj_remove_event_cb(reply,force_original);
    free(expected);
}
int main(int argc,char **argv){
    int benchmark=argc==3&&!strcmp(argv[2],"--benchmark");
    assert(argc==2||benchmark);
    assert(dock_init(NULL,NULL,argv[1])==0);
    /* Keep caret/scrollbar animations from changing pixels between the cached
       and reference renders. Wall-clock timing below remains independent. */
    frozen_ms=tick_ms();lv_tick_set_cb(frozen_tick);
    assert(dock_picoclaw_mount()==0);
    original_glyph=dock_font_body->get_glyph_dsc;dock_font_body->get_glyph_dsc=count_glyph;
    unsigned long short_calls=0;
    for(int paragraph=0;paragraph<(benchmark?1:2);paragraph++)for(int lines=20;lines<=200;lines*=10){
        char *text=calloc(1,(size_t)lines*256),*history=calloc(1,(size_t)lines*256+32);
        assert(text&&history);
        for(int i=0;i<lines;i++){
            if(!benchmark&&!paragraph&&i%17==0)strcat(text,"# 标题 🦞\n");
            strcat(text,"长文本滚动测试，屏幕只需绘制可见的内容，**强调**与 `代码` 保持正常。");
            if(!paragraph)strcat(text,"\n");
        }
        snprintf(history,(size_t)lines*256+32,"a%zu:%s",strlen(text),text);
        dock_picoclaw_render("就绪","","",history,"",0,0,0,1,0);
        lv_obj_scroll_to_y(history_box,LV_COORD_MAX,LV_ANIM_OFF);
        lv_refr_now(display);
        double frames[30];glyph_calls=0;
        int bottom=lv_obj_get_scroll_y(history_box);
        for(int i=0;i<30;i++){
            double start=now_ms();
            lv_obj_scroll_to_y(history_box,bottom-(i%12)*9,LV_ANIM_OFF);
            lv_refr_now(display);
            frames[i]=now_ms()-start;
        }
        qsort(frames,30,sizeof(*frames),compare_ms);
        unsigned long calls=glyph_calls/30;
        printf("paragraph=%d lines=%d frame_median_ms=%.2f frame_p95_ms=%.2f glyph_calls_per_frame=%lu\n",paragraph,lines,frames[15],frames[28],calls);
        fflush(stdout);
        if(lines==20)short_calls=calls;
        else assert(calls<short_calls*2); /* Work follows the viewport, not document length. */
        if(benchmark){free(text);free(history);continue;}
        assert_same_pixels(0);assert_same_pixels(bottom/2);assert_same_pixels(bottom-39);
        lv_obj_t *reply=lv_obj_get_child(lv_obj_get_child(history_box,0),0);
        lv_obj_set_width(reply,460);lv_obj_update_layout(history_box);
        assert_same_pixels(bottom/3);
        dock_theme_appearance(0x0f766e,1);assert_same_pixels(bottom/2);
        dock_theme_appearance(0x0f766e,0);
        /* Updating a later streamed reply keeps earlier widgets and the
           reader's scroll position. Following the bottom resumes there. */
        size_t capacity=strlen(history)+128;
        char *stream=malloc(capacity);assert(stream);
        snprintf(stream,capacity,"%su4:nexta5:hello",history);
        dock_picoclaw_render("回复中","","",stream,"",0,0,1,1,0);
        lv_obj_scroll_to_y(history_box,100,LV_ANIM_OFF);
        lv_obj_t *first=lv_obj_get_child(history_box,0);
        int reading_y=lv_obj_get_scroll_y(history_box);
        snprintf(stream,capacity,"%su4:nexta11:hello world",history);
        dock_picoclaw_render("回复中","","",stream,"",0,0,1,1,0);
        assert(lv_obj_get_child(history_box,0)==first);
        assert(lv_obj_get_scroll_y(history_box)==reading_y);
        assert(lv_obj_get_child_count(history_box)==3);
        lv_obj_scroll_to_y(history_box,LV_COORD_MAX,LV_ANIM_OFF);
        snprintf(stream,capacity,"%su4:nexta12:hello world!",history);
        dock_picoclaw_render("回复中","","",stream,"",0,0,1,1,0);
        assert(lv_obj_get_scroll_bottom(history_box)<=1);
        free(stream);
        free(text);free(history);
    }
    dock_font_body->get_glyph_dsc=original_glyph;
    dock_show_catalog();dock_picoclaw_unmount();dock_close();
    return 0;
}
