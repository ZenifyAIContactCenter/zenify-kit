---
title: Changelog
---

<script setup>
import { data } from './changelog.data'
</script>

# Changelog

Danh sách này sinh lúc build từ [GitHub Releases](https://github.com/ZenifyAIContactCenter/zenify-kit/releases), nguồn mà goreleaser ghi khi cắt tag. Không ai sửa tay trang này. Mỗi release goreleaser lại push commit cask lên `main`, site rebuild và trang tự thấy bản mới. Cách cắt release: xem [Cắt release cho zenify-kit](/guides/release#cắt-release-cho-zenify-kit).

<div v-if="data.error" class="warning custom-block">
  <p class="custom-block-title">Không tải được danh sách release</p>
  <p>{{ data.error }}. Bản build này chạy không có mạng tới GitHub. Xem trực tiếp trên <a href="https://github.com/ZenifyAIContactCenter/zenify-kit/releases">GitHub Releases</a>.</p>
</div>

<template v-for="(r, i) in data.releases" :key="r.tag">
  <h2 :id="r.tag">
    <a :href="r.url">{{ r.tag }}</a>
    <span v-if="i === 0" class="VPBadge tip" style="margin-left: 8px; vertical-align: middle">mới nhất</span>
    <a class="header-anchor" :href="'#' + r.tag" aria-hidden="true">&ZeroWidthSpace;</a>
  </h2>
  <p><em>{{ r.date }}</em></p>
  <p v-if="r.groups.length === 0"><em>Không có commit `feat`, `fix` hay `docs` ngoài các commit tự sinh.</em></p>
  <template v-for="g in r.groups" :key="r.tag + g.title">
    <h3>{{ g.title }}</h3>
    <ul>
      <li v-for="l in g.lines" :key="l.sha">
        <code>{{ l.subject }}</code> <a :href="l.url"><small>{{ l.short }}</small></a>
      </li>
    </ul>
  </template>
</template>

<p v-if="!data.error"><small>Sinh lúc {{ data.fetchedAt.slice(0, 16).replace('T', ' ') }} UTC.</small></p>
