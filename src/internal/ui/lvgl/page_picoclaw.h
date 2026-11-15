#ifndef HACHIMODOCK_PAGE_PICOCLAW_H
#define HACHIMODOCK_PAGE_PICOCLAW_H
int dock_picoclaw_mount(void);
int dock_picoclaw_back(void);
/* history: "u"/"a" (user/answer), "p"/"P" (active/completed process),
   followed by decimal length + ":" + raw text, concatenated.
   While recording is true the textarea mirrors the ASR partial; otherwise the
   textarea is refilled whenever input_revision changes. */
void dock_picoclaw_render(const char *status,const char *partial,const char *hint,const char *history,const char *input,long long input_revision,int recording,int busy,int speech_enabled,int speaking);
void dock_picoclaw_submit(void);
void dock_picoclaw_unmount(void);
#endif
