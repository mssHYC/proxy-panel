import { createApp, h, ref } from 'vue'
import {
  DialogRoot, DialogPortal, DialogOverlay, DialogContent, DialogTitle, DialogDescription,
} from 'reka-ui'
import Button from './Button.vue'

export interface ConfirmOptions {
  title?: string
  message: string
  confirmText?: string
  cancelText?: string
  tone?: 'default' | 'danger'
  prompt?: boolean
  inputType?: 'text' | 'password'
  inputPlaceholder?: string
}

/**
 * Imperative confirm. Resolves with `true` (or the prompted string), rejects on cancel.
 */
export function confirm(opts: ConfirmOptions): Promise<true | string> {
  return new Promise((resolve, reject) => {
    const host = document.createElement('div')
    document.body.appendChild(host)

    let resolved = false

    const teardown = () => {
      app.unmount()
      host.remove()
    }

    const open = ref(true)
    const inputValue = ref('')

    const onConfirm = () => {
      resolved = true
      open.value = false
      if (opts.prompt) resolve(inputValue.value)
      else resolve(true)
    }

    const onCancel = () => {
      open.value = false
    }

    const onAfterClose = (v: boolean) => {
      if (v === false) {
        open.value = false
        if (!resolved) reject('cancel')
        setTimeout(teardown, 200)
      }
    }

    const app = createApp({
      render() {
        return h(DialogRoot, {
          open: open.value,
          'onUpdate:open': onAfterClose,
        }, () => h(DialogPortal, null, () => [
          h(DialogOverlay, { class: 'modal__overlay' }),
          h(DialogContent, {
            class: 'modal__content',
            style: { width: '420px' },
            onOpenAutoFocus: (e: Event) => e.preventDefault(),
          }, () => [
            h('header', { class: 'modal__head' }, [
              h(DialogTitle, { class: 'modal__title' }, () => opts.title || (opts.tone === 'danger' ? '请确认' : '提示')),
            ]),
            h(DialogDescription, { class: 'modal__desc' }, () => opts.message),
            opts.prompt
              ? h('div', { class: 'modal__body' }, [
                  h('input', {
                    type: opts.inputType || 'text',
                    placeholder: opts.inputPlaceholder || '',
                    value: inputValue.value,
                    onInput: (e: Event) => (inputValue.value = (e.target as HTMLInputElement).value),
                    class: 'confirm__input',
                  }),
                ])
              : h('div', { style: 'height: 8px' }),
            h('footer', { class: 'modal__foot' }, [
              h(Button, { variant: 'secondary', onClick: onCancel }, () => opts.cancelText || '取消'),
              h(Button, { variant: opts.tone === 'danger' ? 'primary' : 'primary', onClick: onConfirm }, () => opts.confirmText || '确认'),
            ]),
          ]),
        ]))
      },
    })
    app.mount(host)
  })
}
