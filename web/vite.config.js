import { defineConfig } from 'vite'
import path from 'path'

export default defineConfig({
  build: {
    outDir: 'dist',
    lib: {
      entry: path.resolve(__dirname, 'src/editor.js'),
      name: 'SurveyEditor',
      fileName: 'survey-editor',
      formats: ['iife']
    },
    rollupOptions: {
      output: {
        entryFileNames: 'survey-editor.js',
        assetFileNames: 'survey-editor.[ext]'
      }
    },
    minify: false,
    sourcemap: true
  }
})
