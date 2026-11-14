#ifndef DOCK_PAGE_MUSIC_H
#define DOCK_PAGE_MUSIC_H
int dock_music_mount(void);
void dock_music_unmount(void);
int dock_music_back(void);
void dock_music_login(int cooldown,int busy);
void dock_music_render(const char *status,const char *account,const char *title,const char *artist,const char *album,int playing,int paused,int loading,int elapsed,int duration,int volume,const char *volume_error,int logged_in,int busy,int seekable);
void dock_music_cover(const unsigned char *pixels,int width,int height);
void dock_music_lyrics(const char *previous,const char *current,const char *next);
void dock_music_lists_clear(void);
void dock_music_playlist(const char *id,const char *name,int count,int selected);
void dock_music_track(int index,const char *title,const char *artist,int selected);
void dock_music_tracks_page(int page,int count,int total);
#endif
