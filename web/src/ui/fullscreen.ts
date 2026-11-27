import { useEffect, useState } from 'react';

/** Leaves the browser's full screen, if the page is in it. */
function leaveBrowserFullscreen() {
  if (document.fullscreenElement) {
    void document.exitFullscreen();
  }
}

/**
 * Whether a page fills the whole screen, and the switch for it. On, the browser shows the page on the whole screen
 * where it grants that, and the page lays itself over the window either way, so it can leave its surroundings out.
 * Escape, once nothing else on the page takes it, the browser's own way out of the full screen, and leaving the page
 * all turn it off.
 */
export function useFullscreen(): [boolean, (on: boolean) => void] {
  const [on, setOn] = useState(false);
  useEffect(() => {
    if (!on) {
      return undefined;
    }
    const onChange = () => {
      if (!document.fullscreenElement) {
        setOn(false);
      }
    };
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && !event.defaultPrevented) {
        setOn(false);
        leaveBrowserFullscreen();
      }
    };
    document.addEventListener('fullscreenchange', onChange);
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('fullscreenchange', onChange);
      document.removeEventListener('keydown', onKey);
    };
  }, [on]);
  useEffect(() => leaveBrowserFullscreen, []);
  const turn = (next: boolean) => {
    setOn(next);
    if (!next) {
      leaveBrowserFullscreen();
    } else if (document.fullscreenEnabled && !document.fullscreenElement) {
      // Where the browser refuses, the page still lies over the window.
      document.documentElement.requestFullscreen().catch(() => undefined);
    }
  };
  return [on, turn];
}
