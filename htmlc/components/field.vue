<!-- field.vue renders a single hyper.Field as the appropriate HTML input.
     It receives a field scope (from fieldsToScope) via v-bind.

     Type mapping:
       - text, email, tel, url, number, date, password, hidden → <input>
       - select → <select> with <option> elements
       - checkbox → single <input type="checkbox">
       - checkbox-group → group of checkboxes from options
       - textarea → <textarea>

     Computed properties:
       - isInput: true when type is a single-line input type
-->
<template>
  <!-- text, email, tel, url, number, date, password, hidden -->
  <p v-if="isInput">
    <label :for="name">{{ label || name }}</label>
    <input
      :id="name"
      :type="type"
      :name="name"
      :value="value"
      :required="required"
      :readonly="readOnly" />
    <span class="error" v-if="error">{{ error }}</span>
    <span class="help" v-if="help">{{ help }}</span>
  </p>

  <!-- select -->
  <p v-if="type === 'select'">
    <label :for="name">{{ label || name }}</label>
    <select :id="name" :name="name" :required="required">
      <option v-for="opt in options" :key="opt.value"
        :value="opt.value" :selected="opt.selected">
        {{ opt.label }}
      </option>
    </select>
    <span class="error" v-if="error">{{ error }}</span>
  </p>

  <!-- checkbox -->
  <p v-if="type === 'checkbox'">
    <label>
      <input type="checkbox" :name="name" :value="value" :checked="value" />
      {{ label || name }}
    </label>
  </p>

  <!-- checkbox-group (bulk operations) -->
  <div v-if="type === 'checkbox-group'">
    <label v-for="opt in options" :key="opt.value">
      <input type="checkbox" :name="name" :value="opt.value"
        :checked="opt.selected" />
      {{ opt.label }}
    </label>
  </div>

  <!-- textarea -->
  <p v-if="type === 'textarea'">
    <label :for="name">{{ label || name }}</label>
    <textarea :id="name" :name="name" :required="required">{{ value }}</textarea>
    <span class="error" v-if="error">{{ error }}</span>
    <span class="help" v-if="help">{{ help }}</span>
  </p>
</template>
