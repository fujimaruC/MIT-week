<script lang="ts">
  import { t } from '$lib/i18n/index.svelte';
  import { speechAvailable, speak, stopSpeaking } from '$lib/a11y.svelte';
  import Icon from '$lib/components/ui/Icon.svelte';

  interface Props {
    text: string;
  }

  let { text }: Props = $props();

  const supported = $derived(speechAvailable());
  let speaking = $state(false);

  function toggle(): void {
    if (speaking) {
      stopSpeaking();
      speaking = false;
      return;
    }
    speak(text);
    speaking = true;
  }

  // Stop speech when the component goes away, so navigating never leaves a
  // voice reading a page the user has left.
  $effect(() => () => stopSpeaking());
</script>

{#if supported}
  <button type="button" class="btn btn--quiet btn--sm" onclick={toggle} aria-pressed={speaking}>
    <Icon name={speaking ? 'volume-off' : 'speaker'} size={18} />
    {speaking ? t('read.stop') : t('read.start')}
  </button>
{/if}
