/**
 * Survey Editor - Monaco-based editor with JSON/YAML support and schema autocomplete
 * Uses Monaco from CDN to avoid worker bundling issues
 */

// Survey definition JSON Schema - matches internal/models/survey.go
const surveySchema = {
  $schema: 'http://json-schema.org/draft-07/schema#',
  title: 'Survey Definition',
  description: 'OpenMeet survey definition schema',
  type: 'object',
  required: ['questions'],
  properties: {
    questions: {
      type: 'array',
      description: 'List of survey questions (1-50 questions allowed)',
      minItems: 1,
      maxItems: 50,
      items: {
        type: 'object',
        required: ['id', 'text', 'type'],
        properties: {
          id: {
            type: 'string',
            description: 'Unique question identifier (e.g., "q1", "feedback")',
            maxLength: 64
          },
          text: {
            type: 'string',
            description: 'The question text shown to users',
            maxLength: 1000
          },
          type: {
            type: 'string',
            enum: ['single', 'multi', 'text'],
            description: 'Question type: "single" (radio buttons), "multi" (checkboxes), or "text" (free-form input)'
          },
          required: {
            type: 'boolean',
            description: 'Whether this question must be answered (default: false)',
            default: false
          },
          options: {
            type: 'array',
            description: 'Available choices for single/multi questions (2-20 options, required for single/multi types)',
            minItems: 2,
            maxItems: 20,
            items: {
              type: 'object',
              required: ['id', 'text'],
              properties: {
                id: {
                  type: 'string',
                  description: 'Unique option identifier within this question',
                  maxLength: 64
                },
                text: {
                  type: 'string',
                  description: 'The option text displayed to users',
                  maxLength: 500
                }
              }
            }
          }
        }
      }
    },
    anonymous: {
      type: 'boolean',
      description: 'If true, voter identities are hidden in results (default: false)',
      default: false
    },
    startsAt: {
      type: 'string',
      format: 'date-time',
      description: 'When the survey opens for responses (ISO 8601 format, e.g., "2025-01-01T00:00:00Z")'
    },
    endsAt: {
      type: 'string',
      format: 'date-time',
      description: 'When the survey closes for new responses (ISO 8601 format)'
    }
  },
  additionalProperties: false
}

// Example survey templates organized by category
const examples = {
  // ========== MOTORCYCLE CLUB ==========

  // Monthly ride planning - pick destination and date
  'ride-planning': {
    questions: [
      {
        id: 'destination',
        text: 'Where should we ride this month?',
        type: 'single',
        required: true,
        options: [
          { id: 'volcano', text: 'Volcano National Park (180 mi round trip)' },
          { id: 'waipio', text: 'Waipio Valley Lookout (120 mi round trip)' },
          { id: 'southpoint', text: 'South Point / Green Sand Beach (150 mi round trip)' },
          { id: 'kohala', text: 'North Kohala / Pololu Valley (100 mi round trip)' }
        ]
      },
      {
        id: 'date',
        text: 'Which Saturday works best?',
        type: 'single',
        required: true,
        options: [
          { id: 'first', text: 'First Saturday' },
          { id: 'second', text: 'Second Saturday' },
          { id: 'third', text: 'Third Saturday' },
          { id: 'fourth', text: 'Fourth Saturday' }
        ]
      },
      {
        id: 'departure',
        text: 'Preferred departure time?',
        type: 'single',
        required: true,
        options: [
          { id: 'early', text: '7:00 AM - Beat the heat' },
          { id: 'normal', text: '8:30 AM - Standard departure' },
          { id: 'late', text: '10:00 AM - Sleep in a bit' }
        ]
      }
    ]
  },

  // Club dinner menu selection
  'dinner-menu': {
    questions: [
      {
        id: 'entree',
        text: 'Choose your entree for the January dinner meeting:',
        type: 'single',
        required: true,
        options: [
          { id: 'prime-rib', text: 'Prime Rib (10oz) - $45' },
          { id: 'mahi', text: 'Macadamia Crusted Mahi Mahi - $38' },
          { id: 'chicken', text: 'Grilled Chicken Breast - $32' },
          { id: 'veggie', text: 'Vegetable Stir Fry (Vegan) - $28' }
        ]
      },
      {
        id: 'sides',
        text: 'Choose up to 2 sides:',
        type: 'multi',
        required: false,
        options: [
          { id: 'rice', text: 'Garlic Rice' },
          { id: 'potato', text: 'Baked Potato' },
          { id: 'mac', text: 'Mac Salad' },
          { id: 'veggies', text: 'Steamed Vegetables' },
          { id: 'salad', text: 'House Salad' }
        ]
      },
      {
        id: 'dietary',
        text: 'Any dietary restrictions or allergies?',
        type: 'text',
        required: false
      }
    ]
  },

  // Club gear/merchandise order
  'club-gear': {
    questions: [
      {
        id: 'interest',
        text: 'Would you order new club gear if we do a group buy?',
        type: 'single',
        required: true,
        options: [
          { id: 'yes', text: 'Yes, definitely interested' },
          { id: 'maybe', text: 'Maybe, depends on price/style' },
          { id: 'no', text: 'No, I have enough gear' }
        ]
      },
      {
        id: 'items',
        text: 'Which items would you want? (select all)',
        type: 'multi',
        required: false,
        options: [
          { id: 'patch', text: 'Club Patch (back patch)' },
          { id: 'vest', text: 'Riding Vest with Club Logo' },
          { id: 'tshirt', text: 'Club T-Shirt' },
          { id: 'hat', text: 'Baseball Cap' },
          { id: 'sticker', text: 'Helmet/Bike Stickers' }
        ]
      },
      {
        id: 'budget',
        text: 'What\'s your budget for club gear?',
        type: 'single',
        required: false,
        options: [
          { id: 'low', text: 'Under $50' },
          { id: 'mid', text: '$50-100' },
          { id: 'high', text: '$100-200' },
          { id: 'premium', text: 'Over $200 for quality gear' }
        ]
      }
    ]
  },

  // ========== DISCUSSION GROUPS ==========

  // Topic voting for next meeting
  'topic-vote': {
    questions: [
      {
        id: 'topic',
        text: 'What topic should we discuss at next month\'s meeting?',
        type: 'single',
        required: true,
        options: [
          { id: 'ai', text: 'AI Ethics: Should we worry about superintelligence?' },
          { id: 'climate', text: 'Climate Action: Individual vs Systemic Change' },
          { id: 'media', text: 'Media Literacy: Navigating Misinformation' },
          { id: 'community', text: 'Building Community in the Digital Age' }
        ]
      },
      {
        id: 'format',
        text: 'Preferred discussion format?',
        type: 'single',
        required: false,
        options: [
          { id: 'presentation', text: 'Short presentation then open discussion' },
          { id: 'roundtable', text: 'Roundtable - everyone shares thoughts' },
          { id: 'debate', text: 'Structured debate with sides' },
          { id: 'socratic', text: 'Socratic dialogue with questions' }
        ]
      },
      {
        id: 'suggest',
        text: 'Have a different topic idea? Share it here:',
        type: 'text',
        required: false
      }
    ]
  },

  // Meeting RSVP with details
  'meeting-rsvp': {
    questions: [
      {
        id: 'attending',
        text: 'Will you attend the January 15th meeting?',
        type: 'single',
        required: true,
        options: [
          { id: 'yes', text: 'Yes, I\'ll be there' },
          { id: 'maybe', text: 'Maybe - will confirm later' },
          { id: 'no', text: 'Can\'t make it this time' }
        ]
      },
      {
        id: 'guests',
        text: 'Bringing any guests?',
        type: 'single',
        required: false,
        options: [
          { id: 'none', text: 'Just me' },
          { id: 'one', text: 'Bringing 1 guest' },
          { id: 'two', text: 'Bringing 2 guests' }
        ]
      },
      {
        id: 'food',
        text: 'We\'re ordering pupus. Any dietary needs?',
        type: 'multi',
        required: false,
        options: [
          { id: 'none', text: 'No restrictions' },
          { id: 'vegetarian', text: 'Vegetarian' },
          { id: 'vegan', text: 'Vegan' },
          { id: 'gf', text: 'Gluten-free' },
          { id: 'allergies', text: 'Food allergies (specify below)' }
        ]
      },
      {
        id: 'notes',
        text: 'Any allergies or other notes?',
        type: 'text',
        required: false
      }
    ]
  },

  // Speaker/presentation feedback
  'speaker-feedback': {
    anonymous: true,
    questions: [
      {
        id: 'content',
        text: 'How would you rate the content of tonight\'s presentation?',
        type: 'single',
        required: true,
        options: [
          { id: '5', text: 'Excellent - Very engaging and informative' },
          { id: '4', text: 'Good - Learned something new' },
          { id: '3', text: 'Average - Okay but not memorable' },
          { id: '2', text: 'Below average - Not very relevant' },
          { id: '1', text: 'Poor - Did not meet expectations' }
        ]
      },
      {
        id: 'discussion',
        text: 'How was the discussion portion?',
        type: 'single',
        required: true,
        options: [
          { id: 'great', text: 'Great - Everyone participated well' },
          { id: 'good', text: 'Good - Decent conversation' },
          { id: 'dominated', text: 'A few people dominated' },
          { id: 'short', text: 'Wish we had more time' }
        ]
      },
      {
        id: 'liked',
        text: 'What did you like most?',
        type: 'text',
        required: false
      },
      {
        id: 'improve',
        text: 'What could be improved?',
        type: 'text',
        required: false
      }
    ]
  },

  // Book/article selection for book club
  'book-selection': {
    questions: [
      {
        id: 'book',
        text: 'Vote for next month\'s book:',
        type: 'single',
        required: true,
        options: [
          { id: 'thinking', text: 'Thinking, Fast and Slow - Daniel Kahneman' },
          { id: 'sapiens', text: 'Sapiens - Yuval Noah Harari' },
          { id: 'demon', text: 'The Demon-Haunted World - Carl Sagan' },
          { id: 'atomic', text: 'Atomic Habits - James Clear' }
        ]
      },
      {
        id: 'length',
        text: 'How much reading can you commit to monthly?',
        type: 'single',
        required: false,
        options: [
          { id: 'short', text: 'Short - Under 200 pages' },
          { id: 'medium', text: 'Medium - 200-350 pages' },
          { id: 'long', text: 'Long - 350+ pages is fine' },
          { id: 'any', text: 'Doesn\'t matter to me' }
        ]
      },
      {
        id: 'suggestion',
        text: 'Have a book to suggest for future months?',
        type: 'text',
        required: false
      }
    ]
  },

  // ========== GENERAL PURPOSE ==========

  // Simple quick poll
  'quick-poll': {
    questions: [
      {
        id: 'vote',
        text: 'What day works best for everyone?',
        type: 'single',
        required: true,
        options: [
          { id: 'mon', text: 'Monday' },
          { id: 'tue', text: 'Tuesday' },
          { id: 'wed', text: 'Wednesday' },
          { id: 'thu', text: 'Thursday' },
          { id: 'fri', text: 'Friday' }
        ]
      }
    ]
  },

  // Event feedback
  'event-feedback': {
    anonymous: true,
    questions: [
      {
        id: 'overall',
        text: 'How would you rate this event overall?',
        type: 'single',
        required: true,
        options: [
          { id: '5', text: 'Excellent' },
          { id: '4', text: 'Good' },
          { id: '3', text: 'Average' },
          { id: '2', text: 'Below Average' },
          { id: '1', text: 'Poor' }
        ]
      },
      {
        id: 'venue',
        text: 'How was the venue?',
        type: 'single',
        required: true,
        options: [
          { id: 'great', text: 'Great location and setup' },
          { id: 'good', text: 'Good, worked well' },
          { id: 'ok', text: 'Okay, could be better' },
          { id: 'poor', text: 'Not a good fit' }
        ]
      },
      {
        id: 'again',
        text: 'Would you attend a similar event?',
        type: 'single',
        required: true,
        options: [
          { id: 'definitely', text: 'Definitely' },
          { id: 'probably', text: 'Probably' },
          { id: 'unlikely', text: 'Unlikely' },
          { id: 'no', text: 'No' }
        ]
      },
      {
        id: 'comments',
        text: 'Any other feedback?',
        type: 'text',
        required: false
      }
    ]
  },

  // Volunteer signup
  'volunteer-signup': {
    questions: [
      {
        id: 'help',
        text: 'Can you help with the upcoming event?',
        type: 'single',
        required: true,
        options: [
          { id: 'yes', text: 'Yes, count me in!' },
          { id: 'maybe', text: 'Maybe, depends on the task' },
          { id: 'no', text: 'Not this time' }
        ]
      },
      {
        id: 'tasks',
        text: 'What tasks can you help with? (select all)',
        type: 'multi',
        required: false,
        options: [
          { id: 'setup', text: 'Setup / Decorations' },
          { id: 'food', text: 'Food preparation / serving' },
          { id: 'registration', text: 'Registration / Check-in' },
          { id: 'cleanup', text: 'Cleanup' },
          { id: 'photos', text: 'Photography' },
          { id: 'transport', text: 'Transportation / Logistics' }
        ]
      },
      {
        id: 'hours',
        text: 'How many hours can you volunteer?',
        type: 'single',
        required: false,
        options: [
          { id: '1-2', text: '1-2 hours' },
          { id: '3-4', text: '3-4 hours' },
          { id: 'half', text: 'Half day' },
          { id: 'full', text: 'Full day' }
        ]
      }
    ]
  }
}

// Example categories for UI organization
const exampleCategories = {
  'Motorcycle Club': ['ride-planning', 'dinner-menu', 'club-gear'],
  'Discussion Groups': ['topic-vote', 'meeting-rsvp', 'speaker-feedback', 'book-selection'],
  'General': ['quick-poll', 'event-feedback', 'volunteer-signup']
}

// Human-readable names for examples
const exampleNames = {
  'ride-planning': 'Monthly Ride Planning',
  'dinner-menu': 'Dinner Menu Selection',
  'club-gear': 'Club Gear Order',
  'topic-vote': 'Topic Voting',
  'meeting-rsvp': 'Meeting RSVP',
  'speaker-feedback': 'Speaker Feedback',
  'book-selection': 'Book Club Selection',
  'quick-poll': 'Quick Poll',
  'event-feedback': 'Event Feedback',
  'volunteer-signup': 'Volunteer Signup'
}

/**
 * SurveyEditor class - manages Monaco editor instance
 * Must be initialized after Monaco is loaded via CDN
 */
class SurveyEditor {
  constructor(containerId, options = {}) {
    if (typeof monaco === 'undefined') {
      throw new Error('Monaco editor not loaded. Make sure to load Monaco via CDN first.')
    }

    this.container = document.getElementById(containerId)
    if (!this.container) {
      throw new Error(`Container element #${containerId} not found`)
    }

    this.currentFormat = options.format || 'json'
    this.hiddenInput = options.hiddenInput ? document.getElementById(options.hiddenInput) : null
    this.onValidationChange = options.onValidationChange || null

    // Configure JSON Schema validation
    monaco.languages.json.jsonDefaults.setDiagnosticsOptions({
      validate: true,
      allowComments: false,
      schemaValidation: 'error',
      schemas: [
        {
          uri: 'https://survey.openmeet.net/schema/survey.json',
          fileMatch: ['*'],
          schema: surveySchema
        }
      ]
    })

    // Create format toggle buttons
    this.createFormatToggle()

    // Create editor container
    this.editorContainer = document.createElement('div')
    this.editorContainer.style.height = options.height || '400px'
    this.editorContainer.style.border = '1px solid #ddd'
    this.editorContainer.style.borderRadius = '4px'
    this.container.appendChild(this.editorContainer)

    // Initial content
    const defaultContent = options.initialContent || this.getDefaultContent()

    // Create Monaco editor
    this.editor = monaco.editor.create(this.editorContainer, {
      value: defaultContent,
      language: 'json',
      theme: options.theme || 'vs',
      minimap: { enabled: false },
      automaticLayout: true,
      scrollBeyondLastLine: false,
      fontSize: 14,
      lineNumbers: 'on',
      formatOnPaste: true,
      formatOnType: true,
      tabSize: 2,
      wordWrap: 'on'
    })

    // Sync to hidden input on change
    this.editor.onDidChangeModelContent(() => {
      this.syncToHiddenInput()
      this.updateValidationStatus()
    })

    // Initial sync
    this.syncToHiddenInput()

    // Delay initial validation check to let Monaco process
    setTimeout(() => this.updateValidationStatus(), 500)
  }

  createFormatToggle() {
    const toggleContainer = document.createElement('div')
    toggleContainer.style.marginBottom = '8px'
    toggleContainer.style.display = 'flex'
    toggleContainer.style.gap = '8px'
    toggleContainer.style.alignItems = 'center'

    const label = document.createElement('span')
    label.textContent = 'Format: '
    label.style.fontWeight = '600'
    label.style.marginRight = '4px'

    const jsonBtn = document.createElement('button')
    jsonBtn.type = 'button'
    jsonBtn.textContent = 'JSON'
    jsonBtn.className = 'btn btn-sm' + (this.currentFormat === 'json' ? ' btn-primary' : ' btn-secondary')
    jsonBtn.onclick = () => this.setFormat('json')

    const yamlBtn = document.createElement('button')
    yamlBtn.type = 'button'
    yamlBtn.textContent = 'YAML'
    yamlBtn.className = 'btn btn-sm' + (this.currentFormat === 'yaml' ? ' btn-primary' : ' btn-secondary')
    yamlBtn.onclick = () => this.setFormat('yaml')

    const hint = document.createElement('span')
    hint.textContent = '(Ctrl+Space for autocomplete)'
    hint.style.color = '#7f8c8d'
    hint.style.fontSize = '0.85rem'
    hint.style.marginLeft = '1rem'

    this.jsonBtn = jsonBtn
    this.yamlBtn = yamlBtn

    toggleContainer.appendChild(label)
    toggleContainer.appendChild(jsonBtn)
    toggleContainer.appendChild(yamlBtn)
    toggleContainer.appendChild(hint)
    this.container.appendChild(toggleContainer)
  }

  getDefaultContent() {
    return JSON.stringify({
      questions: [
        {
          id: 'q1',
          text: 'Your question here?',
          type: 'single',
          required: true,
          options: [
            { id: 'opt1', text: 'Option 1' },
            { id: 'opt2', text: 'Option 2' }
          ]
        }
      ]
    }, null, 2)
  }

  setFormat(format) {
    if (format === this.currentFormat) return

    // For now, only JSON has full autocomplete support
    // YAML would need monaco-yaml which has bundling issues
    if (format === 'yaml') {
      alert('YAML editing is available but without autocomplete.\nJSON format is recommended for the best editing experience.')
    }

    const content = this.getValue()
    let converted

    try {
      if (format === 'yaml') {
        // JSON to YAML
        const parsed = JSON.parse(content)
        converted = this.toYaml(parsed)
        monaco.editor.setModelLanguage(this.editor.getModel(), 'yaml')
      } else {
        // YAML to JSON - try to parse as JSON first, then as simple YAML
        let parsed
        try {
          parsed = JSON.parse(content)
        } catch {
          parsed = this.parseSimpleYaml(content)
        }
        converted = JSON.stringify(parsed, null, 2)
        monaco.editor.setModelLanguage(this.editor.getModel(), 'json')
      }
    } catch (e) {
      alert(`Cannot convert: ${e.message}\nPlease fix syntax errors first.`)
      return
    }

    this.currentFormat = format

    // Update button styles
    this.jsonBtn.className = 'btn btn-sm' + (format === 'json' ? ' btn-primary' : ' btn-secondary')
    this.yamlBtn.className = 'btn btn-sm' + (format === 'yaml' ? ' btn-primary' : ' btn-secondary')

    this.editor.setValue(converted)
  }

  // Simple YAML to object parser for survey structure
  parseSimpleYaml(yamlStr) {
    // Try JSON first
    try {
      return JSON.parse(yamlStr)
    } catch {}

    const lines = yamlStr.split('\n')
    const result = { questions: [] }
    let currentQuestion = null
    let currentOptions = []
    let inQuestions = false
    let inOptions = false

    for (const line of lines) {
      const trimmed = line.trim()
      if (!trimmed || trimmed.startsWith('#')) continue

      const indent = line.search(/\S/)

      if (trimmed.startsWith('questions:')) {
        inQuestions = true
        continue
      }
      if (trimmed.startsWith('anonymous:')) {
        result.anonymous = trimmed.includes('true')
        continue
      }
      if (trimmed.startsWith('startsAt:')) {
        result.startsAt = trimmed.split(':', 2)[1].trim().replace(/['"]/g, '')
        continue
      }
      if (trimmed.startsWith('endsAt:')) {
        result.endsAt = trimmed.split(':', 2)[1].trim().replace(/['"]/g, '')
        continue
      }

      if (inQuestions && trimmed.startsWith('- id:')) {
        if (currentQuestion) {
          if (currentOptions.length > 0) currentQuestion.options = currentOptions
          result.questions.push(currentQuestion)
        }
        currentQuestion = { id: this.extractValue(trimmed, '- id:') }
        currentOptions = []
        inOptions = false
      } else if (currentQuestion) {
        if (trimmed.startsWith('text:')) {
          currentQuestion.text = this.extractValue(trimmed, 'text:')
        } else if (trimmed.startsWith('type:')) {
          currentQuestion.type = this.extractValue(trimmed, 'type:')
        } else if (trimmed.startsWith('required:')) {
          currentQuestion.required = trimmed.includes('true')
        } else if (trimmed.startsWith('options:')) {
          inOptions = true
        } else if (inOptions && trimmed.startsWith('- id:')) {
          currentOptions.push({ id: this.extractValue(trimmed, '- id:') })
        } else if (inOptions && trimmed.startsWith('text:') && currentOptions.length > 0) {
          currentOptions[currentOptions.length - 1].text = this.extractValue(trimmed, 'text:')
        }
      }
    }

    if (currentQuestion) {
      if (currentOptions.length > 0) currentQuestion.options = currentOptions
      result.questions.push(currentQuestion)
    }

    return result
  }

  extractValue(line, prefix) {
    return line.replace(prefix, '').trim().replace(/^["']|["']$/g, '')
  }

  // Simple object to YAML serializer
  toYaml(obj, indent = 0) {
    const spaces = '  '.repeat(indent)
    let yaml = ''

    for (const [key, value] of Object.entries(obj)) {
      if (value === undefined || value === null) continue

      if (key === 'questions' && Array.isArray(value)) {
        yaml += `${spaces}questions:\n`
        for (const q of value) {
          yaml += `${spaces}  - id: ${q.id}\n`
          yaml += `${spaces}    text: "${q.text}"\n`
          yaml += `${spaces}    type: ${q.type}\n`
          if (q.required !== undefined) yaml += `${spaces}    required: ${q.required}\n`
          if (q.options && q.options.length > 0) {
            yaml += `${spaces}    options:\n`
            for (const opt of q.options) {
              yaml += `${spaces}      - id: ${opt.id}\n`
              yaml += `${spaces}        text: "${opt.text}"\n`
            }
          }
        }
      } else if (typeof value === 'boolean') {
        yaml += `${spaces}${key}: ${value}\n`
      } else if (typeof value === 'string') {
        yaml += `${spaces}${key}: "${value}"\n`
      }
    }

    return yaml
  }

  getValue() {
    return this.editor.getValue()
  }

  setValue(content) {
    this.editor.setValue(content)
  }

  loadExample(exampleName) {
    const example = examples[exampleName]
    if (!example) {
      console.warn(`Example "${exampleName}" not found`)
      return
    }

    // Always load as JSON for best autocomplete support
    if (this.currentFormat === 'yaml') {
      this.setFormat('json')
    }
    this.editor.setValue(JSON.stringify(example, null, 2))
  }

  syncToHiddenInput() {
    if (this.hiddenInput) {
      this.hiddenInput.value = this.editor.getValue()
    }
  }

  updateValidationStatus() {
    const model = this.editor.getModel()
    if (!model) return

    const markers = monaco.editor.getModelMarkers({ resource: model.uri })
    const errors = markers.filter(m => m.severity === monaco.MarkerSeverity.Error)

    if (this.onValidationChange) {
      this.onValidationChange(errors.length === 0, errors)
    }
  }

  getErrors() {
    const model = this.editor.getModel()
    if (!model) return []

    const markers = monaco.editor.getModelMarkers({ resource: model.uri })
    return markers.filter(m => m.severity === monaco.MarkerSeverity.Error)
  }

  hasErrors() {
    return this.getErrors().length > 0
  }

  format() {
    this.editor.getAction('editor.action.formatDocument').run()
  }

  dispose() {
    this.editor.dispose()
  }
}

// Export for global use
window.SurveyEditor = SurveyEditor
window.surveyExamples = examples
window.surveyExampleCategories = exampleCategories
window.surveyExampleNames = exampleNames
