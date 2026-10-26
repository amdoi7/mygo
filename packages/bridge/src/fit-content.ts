// A window that takes the size of its page's content (FitContent in Go), as
// browsers size the popups of their extensions. The root element is laid out
// at the width of its content (max-content), within the window's limits, so
// that text wraps at the maximum width, and its height is its content's. A
// ResizeObserver on it tells Go the size, with the room the scrollbars take,
// whenever it changes; Go clamps it, turns it into DIPs and resizes the
// window.

/** The limits of the window's size, in the CSS pixels of the page; 0 is
 * none. */
export interface FitLimits {
  minWidth: number;
  minHeight: number;
  maxWidth: number;
  maxHeight: number;
}

/** The rule that lays out the root element at its content's size within the
 * limits, leaving the room of a vertical scrollbar out of the maximum width,
 * so that the root and the scrollbar fit in it. It has no specificity, so that
 * a page's own size for html wins. */
export function fitRule(l: FitLimits, scrollbar: number): string {
  const decls = ["width:max-content"];
  if (l.minWidth > 0) decls.push(`min-width:${Math.max(l.minWidth - scrollbar, 0)}px`);
  if (l.maxWidth > 0) decls.push(`max-width:${Math.max(l.maxWidth - scrollbar, 0)}px`);
  if (l.minHeight > 0) decls.push(`min-height:${l.minHeight}px`);
  return `:where(:root){${decls.join(";")}}`;
}

/** Returns a function that tells Go of a size only when it changed. */
export function sizeReporter(post: (width: number, height: number) => void) {
  let lastWidth = -1;
  let lastHeight = -1;
  return (width: number, height: number) => {
    if (width === lastWidth && height === lastHeight) return;
    lastWidth = width;
    lastHeight = height;
    post(width, height);
  };
}

/** Fits the window to the document's content: the rule applies from now on;
 * start, once the DOM is ready, measures the content and watches it.
 * setLimits takes other limits, as the page's zoom changed. */
export function fitContent(
  limits: FitLimits,
  setRule: (css: string) => void,
  post: (width: number, height: number) => void,
) {
  const report = sizeReporter(post);
  let scrollbar = 0; // the vertical scrollbar's width the rule leaves out
  let started = false;
  let pending = false;
  const apply = () => setRule(fitRule(limits, scrollbar));

  // fromObserver: changing the root's size in a ResizeObserver's callback
  // would end its loop with an error event, which the page would hear; the
  // rule changes in the next frame then, and the observer tells the size.
  const measure = (fromObserver: boolean) => {
    const root = document.documentElement;
    if (!root) return;
    const viewport = document.scrollingElement ?? root;
    const sw = Math.max(innerWidth - viewport.clientWidth, 0);
    const sh = Math.max(innerHeight - viewport.clientHeight, 0);
    if (sw !== scrollbar) {
      scrollbar = sw;
      if (!fromObserver) apply();
      else if (!pending) {
        pending = true;
        requestAnimationFrame(() => {
          pending = false;
          apply();
        });
      }
    }
    const r = root.getBoundingClientRect();
    report(r.width + sw, r.height + sh);
  };

  apply();
  return {
    start() {
      if (started) return;
      started = true;
      measure(false);
      if (typeof ResizeObserver !== "undefined" && document.documentElement) {
        new ResizeObserver(() => measure(true)).observe(document.documentElement);
      }
      // The window's own size changed what scrollbars show.
      addEventListener("resize", () => measure(false));
    },
    setLimits(l: FitLimits) {
      limits = l;
      apply();
      if (started) measure(false);
    },
  };
}
