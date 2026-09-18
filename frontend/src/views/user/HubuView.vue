<template>
  <!-- HUBU（湖北大学）品牌壳：实现见共用组件，文案/色值/图片由品牌注册表驱动 -->
  <CampusConnectView :brand="brands.hubu" />
</template>

<script setup lang="ts">
import CampusConnectView from '@/components/campus/CampusConnectView.vue'
import { brands } from '@/brand'
</script>
