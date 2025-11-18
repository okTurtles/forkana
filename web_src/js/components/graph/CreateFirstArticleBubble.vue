<script setup lang="ts">
const props = defineProps<{
  owner: string | null;
  repo: string | null;
  subject?: string | null;
}>();

function handleCreateArticle(event: Event) {
  event.preventDefault();
  event.stopPropagation();
  
  if (!props.owner || !props.repo) {
    return;
  }
  
  const appSubUrl = (window as any).config?.appSubUrl || '';
  const defaultBranch = 'main';
  const createUrl = `${appSubUrl}/${encodeURIComponent(props.owner)}/${encodeURIComponent(props.repo)}/_new/${defaultBranch}/README.md`;
  
  const isSignedIn = document.body.classList.contains('signed-in');
  
  if (!isSignedIn) {
    const loginUrl = `${appSubUrl}/user/login?redirect_to=${encodeURIComponent(createUrl)}`;
    window.location.href = loginUrl;
  } else {
    window.location.href = createUrl;
  }
}
</script>

<template>
  <div class="state-overlay empty-state">
    <button 
      type="button"
      class="create-article-bubble-button"
      @click="handleCreateArticle"
      aria-label="Create the first article"
    >
      <svg
        class="empty-state-svg"
        viewBox="0 0 320 320"
        preserveAspectRatio="xMidYMid meet"
        aria-hidden="true"
      >
        <defs>
          <radialGradient id="createBubbleGrad" cx="35%" cy="30%" r="65%">
            <stop offset="0%" stop-color="#FAFBFC"/>
            <stop offset="60%" stop-color="#EEF2F7"/>
            <stop offset="100%" stop-color="#E6EBF2"/>
          </radialGradient>
          <filter id="createSoftShadow" x="-50%" y="-50%" width="200%" height="200%">
            <feDropShadow dx="0" dy="2" stdDeviation="3" flood-color="#64748b" flood-opacity="0.18"/>
          </filter>
        </defs>
        
        <circle
          cx="160"
          cy="160"
          r="145" 
          fill="url(#createBubbleGrad)" 
          stroke="#DBE2EA" 
          stroke-width="2" 
          stroke-dasharray="8,6"
          filter="url(#createSoftShadow)"
          class="create-bubble-circle"
        />
        
        <g transform="translate(160, 135)">
          <line x1="0" y1="-25" x2="0" y2="25" stroke="#000000" stroke-width="4" stroke-linecap="round" />
          <line x1="-25" y1="0" x2="25" y2="0" stroke="#000000" stroke-width="4" stroke-linecap="round" />
        </g>
        
        <text
          x="160"
          y="215"
          text-anchor="middle" 
          dominant-baseline="central" 
          fill="#000000" 
          font-size="14" 
          font-weight="500"
          class="create-text"
        >
          Create the first article
        </text>
      </svg>
    </button>
  </div>
</template>

<style scoped>
.state-overlay {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 10;
  padding: 2rem;
  pointer-events: none;
}

.empty-state {
  background-color: rgba(255, 255, 255, 0.98);
  pointer-events: auto !important;
}

.create-article-bubble-button {
  background: none;
  border: none;
  padding: 0;
  cursor: pointer;
  width: 320px;
  height: 320px;
  transition: transform 0.2s ease;
  outline: none;
  pointer-events: auto;
}

.create-article-bubble-button:focus {
  outline: 2px solid var(--color-primary, #7c3aed);
  outline-offset: 4px;
  border-radius: 50%;
}

.create-bubble-circle {
  transition: stroke 0.2s ease, stroke-width 0.2s ease;
}

.create-article-bubble-button:hover .create-bubble-circle,
.create-article-bubble-button:focus .create-bubble-circle {
  stroke: var(--color-primary, #7c3aed);
  stroke-width: 2.5;
}

.plus-symbol line {
  transition: stroke 0.2s ease;
}

.create-article-bubble-button:hover .plus-symbol line,
.create-article-bubble-button:focus .plus-symbol line {
  stroke: var(--color-primary, #7c3aed);
}

.create-text {
  transition: fill 0.2s ease, font-weight 0.2s ease;
}

.create-article-bubble-button:hover .create-text,
.create-article-bubble-button:focus .create-text {
  fill: var(--color-primary, #7c3aed);
  font-weight: 600;
}

.create-article-bubble-button:active {
  transform: scale(0.98);
}

.empty-state-svg {
  width: 100%;
  height: 100%;
  pointer-events: none;
}
</style>
