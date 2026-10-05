<script setup lang="ts">
import {computed} from 'vue';

/* CompareAnnouncement.vue
   The Compare mode banner (#421 item 1): figma "Announcement" (component
   484:190672), drawn under the navbar and above the subject. One component for
   every width; on a phone it is sticky at the top of the page.

   States, each from its figma frame:
     select       "Select 2 articles to compare (0/2 selected)."  641:61765
     one          "1/2 selected – select one more to compare."      641:61848
     ready        "2/2 selected – ready to compare."                641:61937
     unavailable  "No forks yet. Compare needs at least 2 articles.
                   Fork this article to start comparing."           6661:53348
   Every state has the info icon and the dismiss x (both in the brand indigo).
   The x exits compare mode, except in `unavailable`, which is shown while
   compare mode is off: there it only dismisses the notice, and says so.

   The banner is only ever a message: the two picked articles and the action
   live in the Compare box (a popover beside the bubbles, or a bottom sheet on
   a phone). Figma hides the banner's "Compare now" while that box is open
   (641:61937); once the box is closed, `showCompareNow` brings it back so the
   comparison is still one tap away. */

export type CompareAnnouncementState = 'select' | 'one' | 'ready' | 'unavailable';
export type CompareAnnouncementMessages = {
  select: string;
  one: string;
  ready: string;
  unavailable: string;
  compareNow: string;
  /** The x's label while compare mode is on: it exits compare mode. */
  dismiss: string;
  /** The x's label on the `unavailable` notice, which it only hides. */
  dismissNotice: string;
};

const props = withDefaults(defineProps<{
  state: CompareAnnouncementState;
  messages: CompareAnnouncementMessages;
  /** Offer "Compare now" (two articles picked and the Compare box closed). */
  showCompareNow?: boolean;
}>(), {
  showCompareNow: false,
});

const emit = defineEmits<{
  (e: 'dismiss'): void;
  (e: 'compare'): void;
}>();

const message = computed(() => props.messages[props.state]);
const dismissLabel = computed(() => props.state === 'unavailable' ? props.messages.dismissNotice : props.messages.dismiss);
</script>

<template>
  <!-- role="status" is a polite live region already -->
  <div class="compare-announcement" role="status">
    <!-- IconWrapper: octicon info, 16px, brand indigo -->
    <span class="compare-announcement-icon" aria-hidden="true">
      <svg viewBox="0 0 16 16" width="16" height="16"><path fill="currentColor" d="M0 8a8 8 0 1 1 16 0A8 8 0 0 1 0 8Zm8-6.5a6.5 6.5 0 1 0 0 13 6.5 6.5 0 0 0 0-13ZM6.5 7.75A.75.75 0 0 1 7.25 7h1a.75.75 0 0 1 .75.75v2.75h.25a.75.75 0 0 1 0 1.5h-2a.75.75 0 0 1 0-1.5h.25v-2h-.25a.75.75 0 0 1-.75-.75ZM8 6a1 1 0 1 1 0-2 1 1 0 0 1 0 2Z"/></svg>
    </span>
    <p class="compare-announcement-message">{{ message }}</p>
    <div class="compare-announcement-actions">
      <button
        v-if="showCompareNow && state === 'ready'" type="button" class="compare-announcement-primary"
        @click="emit('compare')"
      >
        {{ messages.compareNow }}
      </button>
      <button type="button" class="compare-announcement-dismiss" :aria-label="dismissLabel" @click="emit('dismiss')">
        <svg viewBox="0 0 16 16" width="16" height="16" aria-hidden="true"><path fill="currentColor" d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.749.749 0 0 1 1.275.326.749.749 0 0 1-.215.734L9.06 8l3.22 3.22a.749.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215L8 9.06l-3.22 3.22a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z"/></svg>
      </button>
    </div>
  </div>
</template>


<style scoped>
/* Announcement 484:190672: a tinted strip with a 1px brand border above and
   below (#e9e7ff and #4e40fa @40% in figma). The tint is the brand colour at
   12% over the page, which is #e9e7ff in the light theme and stays a tint of
   the brand on the dark one. */
.compare-announcement {
  box-sizing: border-box;
  width: 100%;
  /* Vertically 7px + the 1px borders: 48px tall, as figma (its strokes are
     inside). Horizontally the content lines up with the navbar, not with
     figma's 48px (figma's navbar is inset 48px, the app's is not): the info
     icon starts where the logo does (--navbar-content-inset-x), and the x's
     glyph ends where the navbar's last item does (--navbar-end-inset-x,
     web_src/css/modules/navbar.css). The x glyph ends 11.5px inside its
     button: 8px of padding plus 3.5px inside the 16px octicon (its rounded
     ends reach 12.5). */
  padding: 7px calc(var(--navbar-end-inset-x) - 11.5px) 7px var(--navbar-content-inset-x);
  border-top: 1px solid color-mix(in srgb, var(--color-primary) 40%, transparent);
  border-bottom: 1px solid color-mix(in srgb, var(--color-primary) 40%, transparent);
  background: color-mix(in srgb, var(--color-primary) 12%, var(--color-body));
  color: var(--color-text-primary);
  display: flex;
  align-items: flex-start;
}

/* IconWrapper: 24×32, the 16px icon centred on the first line */
.compare-announcement-icon {
  display: inline-flex;
  flex-shrink: 0;
  width: 24px;
  padding: 8px 0;
  color: var(--color-primary);
}

/* Content: 6px 8px 6px 0 around Inter 400 14/20 */
.compare-announcement-message {
  flex: 1;
  min-width: 0;
  margin: 0;
  padding: 6px 8px 6px 0;
  font-size: 14px;
  font-weight: 400;
  line-height: 20px;
}

/* Actions: a 4px gap, the primary action then the 32×32 dismiss */
.compare-announcement-actions {
  display: flex;
  flex-shrink: 0;
  align-items: center;
  gap: 4px;
}

/* Primary 484:189112: 32px tall, 6px 12px, radius 6, brand fill with a 1px
   #1f2328 @15% border, Inter 600 14/20 in white */
.compare-announcement-primary {
  height: 32px;
  padding: 6px 12px;
  border: 1px solid rgba(31, 35, 40, 0.15);
  border-radius: 6px;
  background: var(--color-primary);
  box-shadow: 0 1px 0 0 rgba(31, 35, 40, 0.04);
  font-size: 14px;
  font-weight: 600;
  line-height: 20px;
  color: var(--color-primary-contrast);
  white-space: nowrap;
  cursor: pointer;
}

.compare-announcement-primary:hover {
  background: var(--color-primary-hover);
}

/* Dismiss: 32×32, 8px around a 16px x in the brand colour */
.compare-announcement-dismiss {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  padding: 8px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--color-primary);
  cursor: pointer;
}

.compare-announcement-dismiss:hover {
  background: color-mix(in srgb, var(--color-primary) 10%, transparent);
}

.compare-announcement-primary:focus-visible,
.compare-announcement-dismiss:focus-visible {
  outline: 2px solid var(--color-primary);
  outline-offset: 1px;
}

/* Phone (figma Mobile frames 641-63036 / 641-63125 / 6484-44441): edge to
   edge (the side padding follows the navbar, as above) and sticky at the
   top of the page, so the selection stays in sight while the graph is
   scrolled. */
@media (max-width: 767.98px) {
  .compare-announcement {
    position: sticky;
    top: 0;
    z-index: 30;
  }
}
</style>
