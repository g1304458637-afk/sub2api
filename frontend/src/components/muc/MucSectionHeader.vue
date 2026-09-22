<template>
  <div class="muc-section">
    <div class="muc-section__text">
      <h2 class="muc-section__title">
        <span v-if="accent" class="muc-section__accent" aria-hidden="true"></span>
        {{ title }}
      </h2>
      <p v-if="description" class="muc-section__desc">{{ description }}</p>
    </div>
    <div v-if="$slots.actions" class="muc-section__actions">
      <slot name="actions"></slot>
    </div>
  </div>
</template>

<script setup lang="ts">
withDefaults(
  defineProps<{
    title: string
    description?: string
    /** 标题前的民大红竖条（身份感点缀，默认开） */
    accent?: boolean
  }>(),
  { description: '', accent: true }
)
</script>

<style scoped>
.muc-section {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 16px;
}

.muc-section__title {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 17px;
  font-weight: 700;
  letter-spacing: 0.02em;
  color: var(--muc-text-primary);
}

.muc-section__accent {
  width: 3px;
  height: 16px;
  border-radius: 2px;
  background: linear-gradient(to bottom, var(--muc-red-bright), var(--muc-red-deep));
}

.muc-section__desc {
  margin-top: 4px;
  font-size: 12.5px;
  color: var(--muc-text-secondary);
}

.muc-section__actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-shrink: 0;
}
</style>
