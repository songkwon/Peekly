import { useState } from 'react'
import * as Select from '@radix-ui/react-select'
import { Check, ChevronDown, ChevronUp } from 'lucide-react'

type Option<Value extends string> = { value: Value, label: string }

type Props<Value extends string> = {
  id?: string
  'aria-label'?: string
  className?: string
  options: readonly (Value | Option<Value>)[]
  value?: Value
  onChange: (value: Value) => void
  placeholder?: string
  disabled?: boolean
}

export function SelectField<Value extends string>({
  id, 'aria-label': label, className = '', options, value, onChange, placeholder, disabled,
}: Props<Value>) {
  const [dialog, setDialog] = useState<HTMLDialogElement | null>(null)
  // An empty option is a valid filter value. Radix reserves the empty string for placeholders.
  return <Select.Root value={value === undefined ? '' : `:${value}`} disabled={disabled}
    onValueChange={next => onChange(next.slice(1) as Value)}>
    <Select.Trigger id={id} aria-label={label} className={`select-trigger ${className}`}
      ref={node => { if (node) setDialog(node.closest('dialog')) }}>
      <Select.Value placeholder={placeholder} />
      <Select.Icon asChild><ChevronDown size={13} /></Select.Icon>
    </Select.Trigger>
    <Select.Portal container={dialog}>
      <Select.Content position="popper" align="start" sideOffset={4} collisionPadding={10} className="select-menu">
        <Select.ScrollUpButton className="select-scroll"><ChevronUp size={13} /></Select.ScrollUpButton>
        <Select.Viewport className="select-options">
          {options.map(option => {
            const item = typeof option === 'string' ? { value: option, label: option } : option
            return <Select.Item value={`:${item.value}`} key={item.value} className="select-option">
              <Select.ItemText>{item.label}</Select.ItemText>
              <Select.ItemIndicator className="select-indicator"><Check size={13} /></Select.ItemIndicator>
            </Select.Item>
          })}
        </Select.Viewport>
        <Select.ScrollDownButton className="select-scroll"><ChevronDown size={13} /></Select.ScrollDownButton>
      </Select.Content>
    </Select.Portal>
  </Select.Root>
}

export const selectOptions = (labels: Record<string, string>) =>
  Object.entries(labels).map(([value, label]) => ({ value, label }))
