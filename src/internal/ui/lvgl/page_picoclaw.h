#ifndef HACHIMODOCK_PAGE_PICOCLAW_H
#define HACHIMODOCK_PAGE_PICOCLAW_H
int dock_picoclaw_mount(void);
int dock_picoclaw_back(void);
void dock_picoclaw_render(const char *status,const char *partial,const char *history,const char *voice_label,int voice_enabled,const char *audio_status,int busy);
void dock_picoclaw_unmount(void);
#endif
